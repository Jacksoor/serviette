package rest

import (
	"net/http"
	"strconv"

	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/restbridge/auth"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

type Index struct {
	Accounts []string `json:"accounts"`
}

type Account struct {
	Name string                  `json:"name"`
	Info *accountspb.GetResponse `json:"info,omitempty"`
}

type AccountsResource struct {
	authenticator  *auth.Authenticator
	accountsClient accountspb.AccountsClient
}

func NewAccountsResource(authenticator *auth.Authenticator, accountsClient accountspb.AccountsClient) *AccountsResource {
	return &AccountsResource{
		authenticator:  authenticator,
		accountsClient: accountsClient,
	}
}

const maxLimit uint32 = 50

func (r AccountsResource) WebService() *restful.WebService {
	ws := new(restful.WebService)

	ws.Path("/accounts").
		Doc("Account information.").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("").To(r.list).
		Doc("Lists accounts.").
		Writes(Index{}))

	ws.Route(ws.GET("/{accountName}").To(r.read).
		Doc("Reads an account.").
		Param(ws.PathParameter("accountName", "account name")).
		Writes(Account{}))

	return ws
}

func (r AccountsResource) list(req *restful.Request, resp *restful.Response) {
	var offset uint32
	limit := maxLimit

	if rawOffset := req.QueryParameter("offset"); rawOffset != "" {
		v, err := strconv.ParseUint(rawOffset, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad offset")
		}

		offset = uint32(v)
	}

	if rawLimit := req.QueryParameter("limit"); rawLimit != "" {
		v, err := strconv.ParseUint(rawLimit, 10, 32)
		if err != nil {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: bad limit")
		}

		limit = uint32(v)

		if limit > maxLimit {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusBadRequest, "bad request: limit too high")
		}
	}

	listResp, err := r.accountsClient.List(req.Request.Context(), &accountspb.ListRequest{
		Offset: offset,
		Limit:  limit,
	})

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

func (r AccountsResource) read(req *restful.Request, resp *restful.Response) {
	username, err := r.authenticator.Authenticate(req, resp)
	if err != nil {
		glog.Errorf("Failed to authenticate: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	accountName := req.PathParameter("accountName")

	var accountResp *accountspb.GetResponse
	if accountName == username {
		// Fetch extended information.
		var err error
		accountResp, err = r.accountsClient.Get(req.Request.Context(), &accountspb.GetRequest{
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
	}

	resp.WriteEntity(Account{
		Name: accountName,
		Info: accountResp,
	})
}
