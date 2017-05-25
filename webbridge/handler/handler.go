package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/hako/durafmt"
	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	templatePathPrefix string = "webbridge/templates/"
	templatePathSuffix        = ".template.html"
)

type Handler struct {
	*httprouter.Router

	accountsClient accountspb.AccountsClient
	deedsClient    deedspb.DeedsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

var funcMap template.FuncMap = template.FuncMap{
	"prettySeconds": func(seconds int64) string {
		return durafmt.Parse(time.Duration(seconds) * time.Second).String()
	},
}

func New(accountsClient accountspb.AccountsClient, deedsClient deedspb.DeedsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (http.Handler, error) {
	router := httprouter.New()

	h := &Handler{
		Router: router,

		accountsClient: accountsClient,
		deedsClient:    deedsClient,
		moneyClient:    moneyClient,
		scriptsClient:  scriptsClient,
	}

	router.ServeFiles("/static/*filepath", http.Dir("webbridge/static"))

	router.GET("/", h.accountIndex)
	router.POST("/scripts/:scriptAccountHandle", h.scriptCreate)
	router.GET("/scripts/:scriptAccountHandle/:scriptName", h.scriptGet)
	router.POST("/scripts/:scriptAccountHandle/:scriptName", h.scriptUpdate)
	router.POST("/scripts/:scriptAccountHandle/:scriptName/delete", h.scriptDelete)
	router.POST("/deeds", h.deedCreate)

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

func renderTemplate(w http.ResponseWriter, files []string, data interface{}) {
	firstFile := files[0] + templatePathSuffix
	for i, name := range files {
		files[i] = templatePathPrefix + name + templatePathSuffix
	}

	t, err := template.New(firstFile).Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		glog.Errorf("Failed to parse templates: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, data); err != nil {
		glog.Errorf("Failed to execute template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	buf.WriteTo(w)
}

func (h *Handler) accountIndex(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	now := time.Now()

	accountHandle, accountKey, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	balanceResp, err := h.moneyClient.GetBalance(r.Context(), &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{accountHandle},
	})
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	listResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		glog.Errorf("Failed to get script names: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	deedTypesResp, err := h.deedsClient.GetTypes(r.Context(), &deedspb.GetTypesRequest{})
	if err != nil {
		glog.Errorf("Failed to get deed types: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	renderTemplate(w, []string{"_layout", "accountindex"}, struct {
		NowSeconds    int64
		AccountHandle string
		AccountKey    string
		Balance       int64
		ScriptNames   []string
		DeedTypes     []*deedspb.TypeDefinition
	}{
		now.Unix(),
		base64.RawURLEncoding.EncodeToString(accountHandle),
		base64.RawURLEncoding.EncodeToString(accountKey),
		balanceResp.Balance[0],
		listResp.Name,
		deedTypesResp.Definition,
	})
}

var newScriptTemplate string = `#!/usr/bin/python3 -S

"""
This is an example script using Python.
"""

import sys

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
		RequestedCapabilities: &scriptspb.Capabilities{},
		Content:               []byte(newScriptTemplate),
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

	getRequestedCapsResp, err := h.scriptsClient.GetRequestedCapabilities(r.Context(), &scriptspb.GetRequestedCapabilitiesRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		glog.Errorf("Failed to get script requested capabilities: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	renderTemplate(w, []string{"_layout", "scriptview"}, struct {
		ScriptAccountHandle   string
		ScriptName            string
		ScriptContent         string
		RequestedCapabilities *scriptspb.Capabilities
	}{
		base64.RawURLEncoding.EncodeToString(scriptAccountHandle),
		scriptName,
		string(contentResp.Content),
		getRequestedCapsResp.Capabilities,
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

	withdrawalLimit, err := strconv.ParseInt(r.Form.Get("withdrawal_limit"), 10, 64)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if _, err := h.scriptsClient.Create(r.Context(), &scriptspb.CreateRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
		RequestedCapabilities: &scriptspb.Capabilities{
			BillUsageToExecutingAccount: r.Form.Get("bill_usage_to_executing_account") == "on",
			WithdrawalLimit:             withdrawalLimit,
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

func (h *Handler) deedCreate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, accountKey, err := h.authenticate(w, r)
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

	if _, err := h.deedsClient.Buy(r.Context(), &deedspb.BuyRequest{
		AccountHandle: accountHandle,
		AccountKey:    accountKey,
		Type:          r.Form.Get("type"),
		Name:          r.Form.Get("name"),
		Periods:       periods,
		Content:       []byte(r.Form.Get("content")),
	}); err != nil {
		glog.Errorf("Failed to buy deed: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}
