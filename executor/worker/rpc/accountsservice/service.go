package accountsservice

import (
	"encoding/base64"

	"golang.org/x/net/context"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
)

type Service struct {
	accountsClient accountspb.AccountsClient
}

func New(accountsClient accountspb.AccountsClient) *Service {
	return &Service{
		accountsClient: accountsClient,
	}
}

func (s *Service) Lookup(req *struct {
	UserID string `json:"userID"`
}, resp *string) error {
	resolveResp, err := s.accountsClient.ResolveAlias(context.Background(), &accountspb.ResolveAliasRequest{
		Name: req.UserID,
	})

	if err != nil {
		return err
	}

	*resp = base64.RawURLEncoding.EncodeToString(resolveResp.AccountHandle)
	return nil
}
