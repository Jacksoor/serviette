package webdav

import (
	"encoding/base64"
	"net/http"

	"github.com/golang/glog"
	"golang.org/x/net/webdav"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/executor/scripts"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
)

type Handler struct {
	mounter        *scripts.Mounter
	accountsClient accountspb.AccountsClient
}

func NewHandler(mounter *scripts.Mounter, accountsClient accountspb.AccountsClient) *Handler {
	return &Handler{
		mounter:        mounter,
		accountsClient: accountsClient,
	}
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	accountHandle, _, err := h.authenticate(w, r)
	if err != nil {
		return
	}

	mountPath, err := h.mounter.Mount(accountHandle)
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
