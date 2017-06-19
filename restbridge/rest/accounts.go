package rest

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Index struct {
	Accounts []string `json:"accounts"`
}

type Account struct {
	Name    string       `json:"name"`
	Scripts []string     `json:"scripts"`
	Info    *AccountInfo `json:"info,omitempty"`
}

type AccountInfo struct {
	StorageSize uint64 `json:"storageSize"`
	FreeSize    uint64 `json:"freeSize"`

	TimeLimitSeconds int64 `json:"timeLimitSeconds"`
	MemoryLimit      int64 `json:"memoryLimit"`
	TmpfsSize        int64 `json:"tmpfsSize"`

	AllowMessagingService bool `json:"allowMessagingService"`
	AllowRawOutput        bool `json:"allowRawOutput"`
	AllowNetworkAccess    bool `json:"allowNetworkAccess"`
}

type Script struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
}

type AccountsResource struct {
	tokenSecret []byte

	accountsClient accountspb.AccountsClient
	scriptsClient  scriptspb.ScriptsClient
}

func NewAccountsResource(tokenSecret []byte, accountsClient accountspb.AccountsClient, scriptsClient scriptspb.ScriptsClient) *AccountsResource {
	return &AccountsResource{
		tokenSecret: tokenSecret,

		accountsClient: accountsClient,
		scriptsClient:  scriptsClient,
	}
}

func (a AccountsResource) WebService() *restful.WebService {
	ws := new(restful.WebService)

	ws.Path("/accounts").
		Doc("Account information.").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("").To(a.list).
		Doc("Lists accounts.").
		Writes(Index{}))

	ws.Route(ws.GET("/{accountName}").To(a.read).
		Doc("Reads an account.").
		Param(ws.PathParameter("accountName", "account name")).
		Writes(Account{}))

	ws.Route(ws.GET("/{accountName}/scripts/{scriptName}").To(a.readScript).
		Doc("Reads a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Writes(Script{}))

	ws.Route(ws.POST("/{accountName}/scripts").To(a.createScript).
		Doc("Creates a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Reads(Script{}))

	ws.Route(ws.PUT("/{accountName}/scripts/{scriptName}").To(a.updateScript).
		Doc("Updates a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Reads(Script{}))

	ws.Route(ws.DELETE("/{accountName}/scripts/{scriptName}").To(a.deleteScript).
		Doc("Deletes a script.").
		Param(ws.PathParameter("accountName", "account name")).
		Param(ws.PathParameter("scriptName", "script name")).
		Reads(Script{}))

	return ws
}

func (a AccountsResource) list(req *restful.Request, resp *restful.Response) {
	listResp, err := a.accountsClient.List(req.Request.Context(), &accountspb.ListRequest{})

	if err != nil {
		glog.Errorf("Failed to list accounts: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	resp.WriteEntity(Index{
		Accounts: listResp.Name,
	})
}

func (a AccountsResource) read(req *restful.Request, resp *restful.Response) {
	username, err := a.authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	var accountInfo *AccountInfo
	if accountName == username {
		// Fetch extended information.
		accountResp, err := a.accountsClient.Get(req.Request.Context(), &accountspb.GetRequest{
			Username: accountName,
		})
		if err != nil {
			if grpc.Code(err) == codes.NotFound {
				resp.AddHeader("Content-Type", "text/plain")
				resp.WriteErrorString(http.StatusNotFound, "account not found")
			} else {
				glog.Errorf("Failed to get user: %v", err)
				resp.AddHeader("Content-Type", "text/plain")
				resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
			}
			return
		}

		accountInfo = &AccountInfo{
			StorageSize: accountResp.StorageSize,
			FreeSize:    accountResp.FreeSize,

			TimeLimitSeconds: accountResp.TimeLimitSeconds,
			MemoryLimit:      accountResp.MemoryLimit,
			TmpfsSize:        accountResp.TmpfsSize,

			AllowMessagingService: accountResp.AllowMessagingService,
			AllowRawOutput:        accountResp.AllowRawOutput,
			AllowNetworkAccess:    accountResp.AllowNetworkAccess,
		}
	}

	listResp, err := a.scriptsClient.List(req.Request.Context(), &scriptspb.ListRequest{
		OwnerName: accountName,
	})

	if err != nil {
		glog.Errorf("Failed to list scripts: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	resp.WriteEntity(Account{
		Name:    accountName,
		Scripts: listResp.Name,
		Info:    accountInfo,
	})
}

func (a AccountsResource) readScript(req *restful.Request, resp *restful.Response) {
	accountName := req.PathParameter("accountName")
	scriptName := req.PathParameter("scriptName")

	var description string
	var content string

	var g errgroup.Group

	g.Go(func() error {
		metaResp, err := a.scriptsClient.GetMeta(req.Request.Context(), &scriptspb.GetMetaRequest{
			OwnerName: accountName,
			Name:      scriptName,
		})
		if err != nil {
			return err
		}

		description = metaResp.Meta.Description
		return nil
	})

	if req.QueryParameter("getContent") != "" {
		g.Go(func() error {
			contentResp, err := a.scriptsClient.GetContent(req.Request.Context(), &scriptspb.GetContentRequest{
				OwnerName: accountName,
				Name:      scriptName,
			})
			if err != nil {
				return err
			}

			content = string(contentResp.Content)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to get script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	resp.WriteEntity(Script{
		Name:        scriptName,
		Description: description,
		Content:     content,
	})
}

func (a AccountsResource) authenticate(req *restful.Request, resp *restful.Response) (string, error) {
	authorization := strings.SplitN(req.Request.Header.Get("Authorization"), " ", 2)
	if len(authorization) != 2 || authorization[0] != "Bearer" {
		return "", nil
	}

	token, _ := jwt.ParseWithClaims(authorization[1], &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return a.tokenSecret, nil
	})

	if !token.Valid {
		return "", nil
	}

	claims := token.Claims.(*jwt.StandardClaims)
	return claims.Subject, nil
}

func (a AccountsResource) createScript(req *restful.Request, resp *restful.Response) {
	username, err := a.authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	if username != accountName {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	script := new(Script)
	if err := req.ReadEntity(&script); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if _, err := a.scriptsClient.Create(req.Request.Context(), &scriptspb.CreateRequest{
		OwnerName: accountName,
		Name:      script.Name,
		Meta: &scriptspb.Meta{
			Description: script.Description,
		},
		Content: []byte(strings.Replace(script.Content, "\r", "", -1)),
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.AlreadyExists:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusConflict, "script already exists")
		default:
			glog.Errorf("Failed to create script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	resp.WriteEntity(script)
}

func (a AccountsResource) updateScript(req *restful.Request, resp *restful.Response) {
	username, err := a.authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	if username != accountName {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	scriptName := req.PathParameter("scriptName")

	script := new(Script)
	if err := req.ReadEntity(&script); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if _, err := a.scriptsClient.Delete(req.Request.Context(), &scriptspb.DeleteRequest{
		OwnerName: accountName,
		Name:      scriptName,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to delete script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	if _, err := a.scriptsClient.Create(req.Request.Context(), &scriptspb.CreateRequest{
		OwnerName: accountName,
		Name:      script.Name,
		Meta: &scriptspb.Meta{
			Description: script.Description,
		},
		Content: []byte(strings.Replace(script.Content, "\r", "", -1)),
	}); err != nil {
		glog.Errorf("Failed to create script: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	resp.WriteEntity(script)
}

func (a AccountsResource) deleteScript(req *restful.Request, resp *restful.Response) {
	username, err := a.authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	if username != accountName {
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		return
	}

	scriptName := req.PathParameter("scriptName")

	if _, err := a.scriptsClient.Delete(req.Request.Context(), &scriptspb.DeleteRequest{
		OwnerName: accountName,
		Name:      scriptName,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.InvalidArgument:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "script name invalid")
		case codes.NotFound:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "script not found")
		default:
			glog.Errorf("Failed to delete script: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}
}
