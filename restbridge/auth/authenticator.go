package auth

import (
	"fmt"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/emicklei/go-restful"
)

type Authenticator struct {
	tokenSecret []byte
}

func NewAuthenticator(tokenSecret []byte) *Authenticator {
	return &Authenticator{
		tokenSecret: tokenSecret,
	}
}

func (a *Authenticator) Authenticate(req *restful.Request, resp *restful.Response) (string, error) {
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

	if token == nil || !token.Valid {
		return "", nil
	}

	claims := token.Claims.(*jwt.StandardClaims)
	return claims.Subject, nil
}
