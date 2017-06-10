package webdav

import (
	"net/http"

	"github.com/golang/glog"
	"golang.org/x/net/webdav"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/scripts"
)

type Handler struct {
	mounter  *scripts.Mounter
	accounts *accounts.Store
}

func NewHandler(mounter *scripts.Mounter, accounts *accounts.Store) *Handler {
	return &Handler{
		mounter:  mounter,
		accounts: accounts,
	}
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	username, password, _ := r.BasicAuth()

	if err := h.accounts.Authenticate(r.Context(), username, password); err != nil {
		switch err {
		case accounts.ErrNotFound:
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return "", err
		case accounts.ErrUnauthenticated:
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	mountPath, err := h.mounter.Mount(username)
	if err != nil {
		glog.Errorf("Failed to mount account directory: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	(&webdav.Handler{
		FileSystem: webdav.Dir(mountPath),
		LockSystem: webdav.NewMemLS(),
	}).ServeHTTP(w, r)
}
