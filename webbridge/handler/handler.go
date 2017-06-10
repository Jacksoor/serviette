package handler

import (
	"bytes"
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

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
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

func New(staticPath string, templatePath string, accountsClient accountspb.AccountsClient, scriptsClient scriptspb.ScriptsClient) (http.Handler, error) {
	router := httprouter.New()
	csrfHandler := nosurf.New(router)

	h := &Handler{
		Handler: csrfHandler,

		staticPath:   staticPath,
		templatePath: templatePath,

		accountsClient: accountsClient,
		scriptsClient:  scriptsClient,
	}

	router.ServeFiles("/static/*filepath", http.Dir(staticPath))

	csrfHandler.SetFailureHandler(http.HandlerFunc(h.csrfFailure))
	router.GET("/", h.home)
	router.GET("/scripts/:ownerName", h.scriptAccountIndex)
	router.POST("/scripts/:ownerName", h.scriptCreate)
	router.GET("/scripts/:ownerName/:scriptName", h.scriptGet)
	router.POST("/scripts/:ownerName/:scriptName", h.scriptUpdate)
	router.POST("/scripts/:ownerName/:scriptName/delete", h.scriptDelete)

	return h, nil
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	username, password, _ := r.BasicAuth()

	if _, err := h.accountsClient.Authenticate(r.Context(), &accountspb.AuthenticateRequest{
		Username: username,
		Password: password,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound:
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return "", err
		case codes.PermissionDenied:
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return "", err
		}
		glog.Errorf("Failed to load account: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return "", err
	}

	return username, nil
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

func (h *Handler) csrfFailure(w http.ResponseWriter, r *http.Request) {
	http.Error(w, fmt.Sprintf("Forbidden: %s", nosurf.Reason(r)), http.StatusForbidden)
	return
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	username, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptsListResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
		OwnerName: username,
	})
	if err != nil {
		glog.Errorf("Failed to get script names: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "home"}, struct {
		Username    string
		ScriptNames []string

		CSRFToken string
	}{
		username,
		scriptsListResp.Name,

		nosurf.Token(r),
	})
}

func (h *Handler) scriptAccountIndex(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	ownerName := ps.ByName("ownerName")

	listResp, err := h.scriptsClient.List(r.Context(), &scriptspb.ListRequest{
		OwnerName: ownerName,
	})
	if err != nil {
		glog.Errorf("Failed to get script names: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "scriptaccountindex"}, struct {
		OwnerName string
		Names     []string
	}{
		ownerName,
		listResp.Name,
	})
}

func (h *Handler) scriptCreate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	username, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	if username != ps.ByName("ownerName") {
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
		OwnerName: username,
		Name:      scriptName,
		Meta:      &scriptspb.Meta{},
		Content:   contentBuf.Bytes(),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/scripts/%s/%s", username, scriptName), http.StatusMovedPermanently)
}

func (h *Handler) scriptGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	scriptName := ps.ByName("scriptName")
	ownerName := ps.ByName("ownerName")

	contentResp, err := h.scriptsClient.GetContent(r.Context(), &scriptspb.GetContentRequest{
		OwnerName: ownerName,
		Name:      scriptName,
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
		OwnerName: ownerName,
		Name:      scriptName,
	})
	if err != nil {
		glog.Errorf("Failed to get script meta: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.renderTemplate(w, []string{"_layout", "scriptview"}, struct {
		OwnerName     string
		ScriptName    string
		ScriptContent string
		Meta          *scriptspb.Meta

		CSRFToken string
	}{
		ownerName,
		scriptName,
		string(contentResp.Content),
		getMetaResp.Meta,

		nosurf.Token(r),
	})
}

func (h *Handler) scriptUpdate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	username, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptName := ps.ByName("scriptName")

	if username != ps.ByName("ownerName") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if _, err := h.scriptsClient.Delete(r.Context(), &scriptspb.DeleteRequest{
		OwnerName: username,
		Name:      scriptName,
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
		OwnerName: username,
		Name:      scriptName,
		Meta: &scriptspb.Meta{
			Description: r.Form.Get("description"),
		},
		Content: []byte(strings.Replace(r.Form.Get("content"), "\r", "", -1)),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/scripts/%s/%s", username, scriptName), http.StatusMovedPermanently)
}

func (h *Handler) scriptDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	username, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	scriptName := ps.ByName("scriptName")

	if username != ps.ByName("ownerName") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if _, err := h.scriptsClient.Delete(r.Context(), &scriptspb.DeleteRequest{
		OwnerName: username,
		Name:      scriptName,
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
