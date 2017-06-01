package handler

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/hako/durafmt"
	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	templatePathSuffix = ".template.html"
)

type Handler struct {
	*httprouter.Router

	staticPath   string
	templatePath string

	aliasCost     int64
	aliasDuration time.Duration

	accountsClient accountspb.AccountsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

var funcMap template.FuncMap = template.FuncMap{
	"prettyDuration": func(dur time.Duration) string {
		return durafmt.Parse(dur).String()
	},

	"prettyTime": func(t time.Time) string {
		return durafmt.Parse(t.Sub(time.Now())).String()
	},

	"eq": func(a interface{}, b interface{}) bool {
		return a == b
	},
}

func New(staticPath string, templatePath string, aliasCost int64, aliasDuration time.Duration, accountsClient accountspb.AccountsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (http.Handler, error) {
	router := httprouter.New()

	h := &Handler{
		Router: router,

		staticPath:   staticPath,
		templatePath: templatePath,

		aliasCost:     aliasCost,
		aliasDuration: aliasDuration,

		accountsClient: accountsClient,
		moneyClient:    moneyClient,
		scriptsClient:  scriptsClient,
	}

	router.ServeFiles("/static/*filepath", http.Dir(staticPath))

	router.GET("/", h.home)
	router.GET("/scripts", h.scriptIndex)
	router.GET("/scripts/:scriptAccountHandle", h.scriptAccountIndex)
	router.POST("/scripts/:scriptAccountHandle", h.scriptCreate)
	router.GET("/scripts/:scriptAccountHandle/:scriptName", h.scriptGet)
	router.POST("/scripts/:scriptAccountHandle/:scriptName", h.scriptUpdate)
	router.POST("/scripts/:scriptAccountHandle/:scriptName/delete", h.scriptDelete)
	router.POST("/aliases", h.aliasCreate)
	router.POST("/aliases/:aliasName", h.aliasUpdate)
	router.POST("/aliases/:aliasName/renew", h.aliasRenew)
	router.POST("/aliases/:aliasName/delete", h.aliasDelete)

	return h, nil
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) ([]byte, []byte, error) {
	rawHandle, rawKey, _ := r.BasicAuth()

	accountHandle, err := base64.RawURLEncoding.DecodeString(rawHandle)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, nil, err
	}
	accountKey, err := base64.RawURLEncoding.DecodeString(rawKey)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, nil, err
	}

	if _, err := h.accountsClient.Check(r.Context(), &accountspb.CheckRequest{
		AccountHandle: accountHandle,
		AccountKey:    accountKey,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.PermissionDenied, codes.NotFound:
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		default:
			glog.Errorf("Failed to check account: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return nil, nil, err
	}

	return accountHandle, accountKey, nil
}

func (h *Handler) renderTemplate(w http.ResponseWriter, files []string, data interface{}) {
	firstFile := files[0] + templatePathSuffix
	for i, name := range files {
		files[i] = filepath.Join(h.templatePath, name+templatePathSuffix)
	}

	t, err := template.New(firstFile).Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		glog.Errorf("Failed to parse templates: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		glog.Errorf("Failed to execute template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

type aliasDetails struct {
	Name          string
	AccountHandle string
	ScriptName    string
	ExpiryTime    time.Time
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, accountKey, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	balanceResp, err := h.moneyClient.GetBalance(r.Context(), &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	scriptsListResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get script names: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	aliasesListResp, err := h.scriptsClient.ListAliases(r.Context(), &scriptspb.ListAliasesRequest{})
	if err != nil {
		glog.Errorf("Failed to list aliases: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	aliases := make([]*aliasDetails, 0)
	for _, entry := range aliasesListResp.Entry {
		if string(entry.AccountHandle) != string(accountHandle) {
			continue
		}

		aliases = append(aliases, &aliasDetails{
			Name:          entry.Name,
			AccountHandle: base64.RawURLEncoding.EncodeToString(entry.AccountHandle),
			ScriptName:    entry.ScriptName,
			ExpiryTime:    time.Unix(entry.ExpiryTimeUnix, 0),
		})
	}

	h.renderTemplate(w, []string{"_layout", "home"}, struct {
		AccountHandle string
		AccountKey    string
		Balance       int64
		ScriptNames   []string
		Aliases       []*aliasDetails

		AliasCost     int64
		AliasDuration time.Duration
	}{
		base64.RawURLEncoding.EncodeToString(accountHandle),
		base64.RawURLEncoding.EncodeToString(accountKey),
		balanceResp.Balance,
		scriptsListResp.Name,
		aliases,

		h.aliasCost,
		h.aliasDuration,
	})
}

func (h *Handler) scriptIndex(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	listResp, err := h.accountsClient.List(r.Context(), &accountspb.ListRequest{})
	if err != nil {
		glog.Errorf("Failed to list accounts: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	accountScripts := make(map[string][]string, len(listResp.AccountHandle))
	for _, accountHandle := range listResp.AccountHandle {
		listResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
			AccountHandle: accountHandle,
		})

		if len(listResp.Name) == 0 {
			continue
		}

		if err != nil {
			glog.Errorf("Failed to get script names: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		accountScripts[base64.RawURLEncoding.EncodeToString(accountHandle)] = listResp.Name
	}

	aliasesListResp, err := h.scriptsClient.ListAliases(r.Context(), &scriptspb.ListAliasesRequest{})
	if err != nil {
		glog.Errorf("Failed to list aliases: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	aliases := make([]*aliasDetails, len(aliasesListResp.Entry))
	for i, entry := range aliasesListResp.Entry {
		aliases[i] = &aliasDetails{
			Name:          entry.Name,
			AccountHandle: base64.RawURLEncoding.EncodeToString(entry.AccountHandle),
			ScriptName:    entry.ScriptName,
			ExpiryTime:    time.Unix(entry.ExpiryTimeUnix, 0),
		}
	}

	h.renderTemplate(w, []string{"_layout", "scriptindex"}, struct {
		AccountScripts map[string][]string
		Aliases        []*aliasDetails
	}{
		accountScripts,
		aliases,
	})
}

func (h *Handler) scriptAccountIndex(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(ps.ByName("scriptAccountHandle"))
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	listResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
		AccountHandle: scriptAccountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get script names: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "scriptaccountindex"}, struct {
		ScriptAccountHandle string
		Names               []string
	}{
		base64.RawURLEncoding.EncodeToString(scriptAccountHandle),
		listResp.Name,
	})
}

var newScriptTemplate string = `#!/usr/bin/python3

"""
This is an example script using Python.
"""

import sys
sys.path.insert(0, '/usr/lib/k4')

# Import the k4 library.
import k4


# Input.
inp = sys.stdin.read()

# Open some persistent storage.
try:
    with open('/mnt/storage/number_of_greetings', 'r') as f:
        num_his = int(f.read())
except IOError:
    num_his = 0

# Create a new client.
client = k4.Client()

# Get the current context.
context = client.Context.Get()

# Greet the user!
print('Hi, {}! I\'ve said "hi" {} times! You said "{}"!'.format(
    context['mention'], num_his, inp))

# Increment the number of his and put it back into persistent storage.
with open('/mnt/storage/number_of_greetings', 'w') as f:
    f.write(str(num_his + 1))
`

func (h *Handler) scriptCreate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(ps.ByName("scriptAccountHandle"))
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if string(accountHandle) != string(scriptAccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	scriptName := r.Form.Get("name")

	if _, err := h.scriptsClient.Create(r.Context(), &scriptspb.CreateRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
		Requirements:  &scriptspb.Requirements{},
		Content:       []byte(newScriptTemplate),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/scripts/%s/%s", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName), http.StatusMovedPermanently)
}

func (h *Handler) scriptGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	scriptName := ps.ByName("scriptName")
	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(ps.ByName("scriptAccountHandle"))
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contentResp, err := h.scriptsClient.GetContent(r.Context(), &scriptspb.GetContentRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		glog.Errorf("Failed to get script content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	getRequirementsResp, err := h.scriptsClient.GetRequirements(r.Context(), &scriptspb.GetRequirementsRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		glog.Errorf("Failed to get script requirements: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "scriptview"}, struct {
		ScriptAccountHandle string
		ScriptName          string
		ScriptContent       string
		Requirements        *scriptspb.Requirements
	}{
		base64.RawURLEncoding.EncodeToString(scriptAccountHandle),
		scriptName,
		string(contentResp.Content),
		getRequirementsResp.Requirements,
	})
}

func (h *Handler) scriptUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptName := ps.ByName("scriptName")
	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(ps.ByName("scriptAccountHandle"))
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if string(accountHandle) != string(scriptAccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if _, err := h.scriptsClient.Delete(r.Context(), &scriptspb.DeleteRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	}); err != nil {
		if grpc.Code(err) == codes.NotFound {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		glog.Errorf("Failed to delete script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.scriptsClient.Create(r.Context(), &scriptspb.CreateRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
		Requirements: &scriptspb.Requirements{
			BillUsageToOwner: r.Form.Get("bill_usage_to_owner") == "on",
			NeedsEscrow:      r.Form.Get("needs_escrow") == "on",
		},
		Content: []byte(strings.Replace(r.Form.Get("content"), "\r", "", -1)),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/scripts/%s/%s", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName), http.StatusMovedPermanently)
}

func (h *Handler) scriptDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptName := ps.ByName("scriptName")
	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(ps.ByName("scriptAccountHandle"))
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if string(accountHandle) != string(scriptAccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if _, err := h.scriptsClient.Delete(r.Context(), &scriptspb.DeleteRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	}); err != nil {
		if grpc.Code(err) == codes.NotFound {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		glog.Errorf("Failed to delete script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (h *Handler) aliasCreate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	now := time.Now()

	periods, err := strconv.ParseInt(r.Form.Get("periods"), 10, 64)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	balanceResp, err := h.moneyClient.GetBalance(r.Context(), &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	cost := periods * h.aliasCost
	if balanceResp.Balance < cost {
		http.Error(w, "Forbidden: insufficient funds", http.StatusForbidden)
		return
	}

	if _, err := h.moneyClient.Add(r.Context(), &moneypb.AddRequest{
		AccountHandle: accountHandle,
		Amount:        -cost,
	}); err != nil {
		glog.Errorf("Failed to adjust balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.scriptsClient.SetAlias(r.Context(), &scriptspb.SetAliasRequest{
		Name:           r.Form.Get("name"),
		AccountHandle:  accountHandle,
		ScriptName:     r.Form.Get("script_name"),
		ExpiryTimeUnix: now.Add(time.Duration(periods) * h.aliasDuration).Unix(),
	}); err != nil {
		glog.Errorf("Failed to set alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (h *Handler) aliasUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	resolveResp, err := h.scriptsClient.ResolveAlias(r.Context(), &scriptspb.ResolveAliasRequest{
		Name: ps.ByName("aliasName"),
	})
	if err != nil {
		glog.Errorf("Failed to resolve alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if string(accountHandle) != string(resolveResp.AccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if _, err := h.scriptsClient.SetAlias(r.Context(), &scriptspb.SetAliasRequest{
		Name:           ps.ByName("aliasName"),
		AccountHandle:  accountHandle,
		ScriptName:     r.Form.Get("script_name"),
		ExpiryTimeUnix: resolveResp.ExpiryTimeUnix,
	}); err != nil {
		glog.Errorf("Failed to set alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (h *Handler) aliasRenew(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	periods, err := strconv.ParseInt(r.Form.Get("periods"), 10, 64)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	resolveResp, err := h.scriptsClient.ResolveAlias(r.Context(), &scriptspb.ResolveAliasRequest{
		Name: ps.ByName("aliasName"),
	})
	if err != nil {
		glog.Errorf("Failed to resolve alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if string(accountHandle) != string(resolveResp.AccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	balanceResp, err := h.moneyClient.GetBalance(r.Context(), &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	cost := periods * h.aliasCost
	if balanceResp.Balance < cost {
		http.Error(w, "Forbidden: insufficient funds", http.StatusForbidden)
		return
	}

	if _, err := h.moneyClient.Add(r.Context(), &moneypb.AddRequest{
		AccountHandle: accountHandle,
		Amount:        -cost,
	}); err != nil {
		glog.Errorf("Failed to adjust balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.scriptsClient.SetAlias(r.Context(), &scriptspb.SetAliasRequest{
		Name:           ps.ByName("aliasName"),
		AccountHandle:  accountHandle,
		ScriptName:     resolveResp.ScriptName,
		ExpiryTimeUnix: time.Unix(resolveResp.ExpiryTimeUnix, 0).Add(time.Duration(periods) * h.aliasDuration).Unix(),
	}); err != nil {
		glog.Errorf("Failed to set alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (h *Handler) aliasDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	resolveResp, err := h.scriptsClient.ResolveAlias(r.Context(), &scriptspb.ResolveAliasRequest{
		Name: ps.ByName("aliasName"),
	})
	if err != nil {
		glog.Errorf("Failed to resolve alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if string(accountHandle) != string(resolveResp.AccountHandle) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if _, err := h.scriptsClient.SetAlias(r.Context(), &scriptspb.SetAliasRequest{
		Name:           ps.ByName("aliasName"),
		AccountHandle:  nil,
		ScriptName:     "",
		ExpiryTimeUnix: 0,
	}); err != nil {
		glog.Errorf("Failed to set alias: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}
