package rest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dgrijalva/jwt-go"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

type UserpassCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type DiscordCredentials struct {
	Token             string `json:"token"`
	PreferredUsername string `json:"preferredUsername,omitempty"`
}

type Token struct {
	Username string `json:"username"`
	Token    string `json:"token"`
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

	ws.Route(ws.POST("userpass").To(l.userpass).
		Doc("Log in via username/password.").
		Reads(UserpassCredentials{}).
		Writes([]*Token{}))

	ws.Route(ws.POST("discord").To(l.discord).
		Doc("Log in via Discord credentials.").
		Reads(DiscordCredentials{}).
		Writes([]*Token{}))

	return ws
}

func (l LoginResource) createToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.StandardClaims{
		Subject:   username,
		ExpiresAt: time.Now().Add(l.tokenDuration).Unix(),
	})

	tokenString, err := token.SignedString(l.tokenSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (l LoginResource) userpass(req *restful.Request, resp *restful.Response) {
	creds := new(UserpassCredentials)
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

	tokenString, err := l.createToken(creds.Username)
	if err != nil {
		glog.Errorf("Failed to create token: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
	}

	resp.WriteEntity([]*Token{
		&Token{
			Username: creds.Username,
			Token:    tokenString,
		},
	})
}

func (l LoginResource) discord(req *restful.Request, resp *restful.Response) {
	creds := new(DiscordCredentials)
	if err := req.ReadEntity(creds); err != nil {
		glog.Errorf("Failed to read entity: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	session, err := discordgo.New("Bearer " + creds.Token)
	if err != nil {
		glog.Errorf("Failed to authenticate with Discord: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}
	session.StateEnabled = false
	user, err := session.User("@me")
	if err != nil {
		glog.Errorf("Failed to get Discord info: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}
	session.Close()

	listResp, err := l.accountsClient.ListByIdentifier(req.Request.Context(), &accountspb.ListByIdentifierRequest{
		Identifier: fmt.Sprintf("discord/%s", user.ID),
	})

	if err != nil {
		glog.Errorf("Failed to get accounts from executor: %v", err)
		resp.AddHeader("Content-Type", "text/plain")
		resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		return
	}

	if len(listResp.Name) == 0 {
		if creds.PreferredUsername == "" {
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusNotFound, "account not found")
			return
		}

		// We can create an account!
		if _, err := l.accountsClient.Create(req.Request.Context(), &accountspb.CreateRequest{
			Username:   creds.PreferredUsername,
			Identifier: []string{fmt.Sprintf("discord/%s", user.ID)},
		}); err != nil {
			glog.Errorf("Failed to create account: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
			return
		}

		tokenString, err := l.createToken(creds.PreferredUsername)
		if err != nil {
			glog.Errorf("Failed to create token: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}

		resp.WriteEntity([]*Token{
			&Token{
				Username: creds.PreferredUsername,
				Token:    tokenString,
			},
		})
		return
	}

	tokens := make([]*Token, 0)

	for _, username := range listResp.Name {
		tokenString, err := l.createToken(username)
		if err != nil {
			glog.Errorf("Failed to create token: %v", err)
			resp.AddHeader("Content-Type", "text/plain")
			resp.WriteErrorString(http.StatusInternalServerError, "internal server error")
		}

		tokens = append(tokens, &Token{
			Username: username,
			Token:    tokenString,
		})
	}

	resp.WriteEntity(tokens)
}
