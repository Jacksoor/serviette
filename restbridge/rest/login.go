package rest

import (
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Token struct {
	Token string `json:"token"`
}

type LoginResource struct {
	tokenSecret   []byte
	tokenDuration time.Duration

	accountsClient accountspb.AccountsClient
}

func NewLoginResource(tokenSecret []byte, tokenDuration time.Duration, accountsClient accountspb.AccountsClient) *LoginResource {
	return &LoginResource{
		tokenSecret:   tokenSecret,
		tokenDuration: tokenDuration,

		accountsClient: accountsClient,
	}
}

func (l LoginResource) WebService() *restful.WebService {
	ws := new(restful.WebService)

	ws.Path("/login").
		Doc("Get account information.").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.POST("").To(l.login).
		Doc("Log in.").
		Reads(Credentials{}).
		Writes(Token{}))

	return ws
}

func (l LoginResource) login(req *restful.Request, resp *restful.Response) {
	creds := new(Credentials)
	if err := req.ReadEntity(creds); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if _, err := l.accountsClient.Authenticate(req.Request.Context(), &accountspb.AuthenticateRequest{
		Username: creds.Username,
		Password: creds.Password,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.PermissionDenied:
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusUnauthorized, "unauthorized")
		default:
			glog.Errorf("Failed to authenticate: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.StandardClaims{
		Subject:   creds.Username,
		ExpiresAt: time.Now().Add(l.tokenDuration).Unix(),
	})

	tokenString, err := token.SignedString(l.tokenSecret)
	if err != nil {
		glog.Errorf("Failed to sign token: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	resp.WriteEntity(Token{
		Token: tokenString,
	})
}
