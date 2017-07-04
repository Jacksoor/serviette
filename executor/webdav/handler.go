package webdav

import (
	"net/http"

	"github.com/golang/glog"
	"golang.org/x/net/webdav"

	"github.com/porpoises/kobun4/executor/accounts"
)

type Handler struct {
	accounts *accounts.Store
}

func NewHandler(accounts *accounts.Store) *Handler {
	return &Handler{
		accounts: accounts,
	}
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (*accounts.Account, error) {
	username, password, _ := r.BasicAuth()

	account, err := h.accounts.Account(r.Context(), username)
	if err != nil {
		if err == accounts.ErrNotFound {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil, err
		}
		glog.Errorf("Failed to load account: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, err
	}

	if err := account.Authenticate(r.Context(), password); err != nil {
		switch err {
		case accounts.ErrUnauthenticated:
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Kobun\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil, err
		}
		glog.Errorf("Failed to authenticate account: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, err
	}

	return account, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	account, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	(&webdav.Handler{
		FileSystem: webdav.Dir(account.PrivateStoragePath()),
		LockSystem: webdav.NewMemLS(),
	}).ServeHTTP(w, r)
}
