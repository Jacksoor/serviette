package handler

import (
	"bytes"
	"encoding/base64"
	"html/template"
	"net/http"
	_ "path/filepath"

	"github.com/golang/glog"
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

	router.GET("/", h.welcome)
	router.GET("/accounts/:accountHandle", h.accountIndex)
	router.GET("/accounts/:accountHandle/scripts/:scriptName", h.scriptView)

	return h, nil
}

func (h *Handler) welcome(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	renderTemplate(w, []string{"_layout", "welcome"}, nil)
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) ([]byte, []byte, error) {
	rawHandle := ps.ByName("accountHandle")
	rawKey := r.URL.Query().Get("key")

	accountHandle, err := base64.RawURLEncoding.DecodeString(rawHandle)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return nil, nil, err
	}
	accountKey, err := base64.RawURLEncoding.DecodeString(rawKey)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return nil, nil, err
	}

	if _, err := h.accountsClient.Check(r.Context(), &accountspb.CheckRequest{
		AccountHandle: accountHandle,
		AccountKey:    accountKey,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.PermissionDenied:
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case codes.NotFound:
			http.Error(w, "Not found", http.StatusNotFound)
		default:
			glog.Errorf("Failed to check account: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return nil, nil, err
	}

	return accountHandle, accountKey, nil
}

func renderTemplate(w http.ResponseWriter, files []string, data interface{}) {
	for i, name := range files {
		files[i] = templatePathPrefix + name + templatePathSuffix
	}
	t, err := template.ParseFiles(files...)

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
	accountHandle, accountKey, err := h.authenticate(w, r, ps)
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

	renderTemplate(w, []string{"_layout", "accountindex"}, struct {
		AccountHandle string
		AccountKey    string
		Balance       int64
	}{
		base64.RawURLEncoding.EncodeToString(accountHandle),
		base64.RawURLEncoding.EncodeToString(accountKey),
		balanceResp.Balance[0],
	})
}

func (h *Handler) scriptView(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	accountHandle, accountKey, err := h.authenticate(w, r, ps)
	if err != nil {
		return
	}

	scriptName := ps.ByName("scriptName")

	renderTemplate(w, []string{"_layout", "scriptview"}, struct {
		AccountHandle string
		AccountKey    string

		ScriptName string
	}{
		base64.RawURLEncoding.EncodeToString(accountHandle),
		base64.RawURLEncoding.EncodeToString(accountKey),
		scriptName,
	})
}
