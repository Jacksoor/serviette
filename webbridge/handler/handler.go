package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/hako/durafmt"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/nosurf"
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
	http.Handler

	staticPath   string
	templatePath string

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

func New(staticPath string, templatePath string, accountsClient accountspb.AccountsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (http.Handler, error) {
	router := httprouter.New()

	h := &Handler{
		Handler: nosurf.New(router),

		staticPath:   staticPath,
		templatePath: templatePath,

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

	getResp, err := h.accountsClient.Get(r.Context(), &accountspb.GetRequest{
		AccountHandle: accountHandle,
	})

	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil, nil, err
		}

		glog.Errorf("Failed to check account: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, nil, err
	}

	if string(getResp.AccountKey) != string(accountKey) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	h.renderTemplate(w, []string{"_layout", "home"}, struct {
		AccountHandle string
		AccountKey    string
		Balance       int64
		ScriptNames   []string

		CSRFToken string
	}{
		base64.RawURLEncoding.EncodeToString(accountHandle),
		base64.RawURLEncoding.EncodeToString(accountKey),
		balanceResp.Balance,
		scriptsListResp.Name,

		nosurf.Token(r),
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

	h.renderTemplate(w, []string{"_layout", "scriptindex"}, struct {
		AccountScripts map[string][]string
	}{
		accountScripts,
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

	var contentBuf bytes.Buffer
	t, err := template.ParseFiles(filepath.Join(h.templatePath, "script.template.py"))
	if err != nil {
		glog.Errorf("Failed to parse script template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := t.Execute(&contentBuf, struct{}{}); err != nil {
		glog.Errorf("Failed to execute script template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.scriptsClient.Create(r.Context(), &scriptspb.CreateRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
		Meta:          &scriptspb.Meta{},
		Content:       contentBuf.Bytes(),
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

	getMetaResp, err := h.scriptsClient.GetMeta(r.Context(), &scriptspb.GetMetaRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		glog.Errorf("Failed to get script meta: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "scriptview"}, struct {
		ScriptAccountHandle string
		ScriptName          string
		ScriptContent       string
		Meta                *scriptspb.Meta

		CSRFToken string
	}{
		base64.RawURLEncoding.EncodeToString(scriptAccountHandle),
		scriptName,
		string(contentResp.Content),
		getMetaResp.Meta,

		nosurf.Token(r),
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
		Meta: &scriptspb.Meta{
			Description:      r.Form.Get("description"),
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
