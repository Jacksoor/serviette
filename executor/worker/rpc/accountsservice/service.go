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

type LookupRequest struct {
	Name string `json:"name"`
}

type LookupResponse struct {
	AccountHandle string `json:"account_handle"`
}

func (s *Service) Lookup(req *LookupRequest, resp *LookupResponse) error {
	resolveResp, err := s.accountsClient.ResolveAlias(context.Background(), &accountspb.ResolveAliasRequest{
		Name: req.Name,
	})

	if err != nil {
		return err
	}

	resp.AccountHandle = base64.RawURLEncoding.EncodeToString(resolveResp.AccountHandle)
	return nil
}
