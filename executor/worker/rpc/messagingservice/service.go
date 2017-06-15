package messagingservice

import (
	"errors"

	"golang.org/x/net/context"

	"github.com/porpoises/kobun4/executor/accounts"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
)

type Service struct {
	ctx       context.Context
	account   *accounts.Account
	messaging messagingpb.MessagingClient
}

func New(ctx context.Context, account *accounts.Account, messaging messagingpb.MessagingClient) *Service {
	return &Service{
		ctx:       ctx,
		account:   account,
		messaging: messaging,
	}
}

func (s *Service) MessageChannel(req *struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Format  string `json:"format"`
}, resp *struct{}) error {
	format := req.Format
	if format == "" {
		format = "text"
	}

	if format == "raw" && !s.account.AllowRawOutput {
		return errors.New("raw format requested but account is not allowed to send raw output")
	}
	if _, err := s.messaging.Message(s.ctx, &messagingpb.MessageRequest{
		Content: []byte(req.Content),
		Format:  format,
		Target: &messagingpb.MessageRequest_ChannelId{
			ChannelId: req.ID,
		},
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) MessageUser(req *struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Format  string `json:"format"`
}, resp *struct{}) error {
	format := req.Format
	if format == "" {
		format = "text"
	}

	if format == "raw" && !s.account.AllowRawOutput {
		return errors.New("raw format requested but account is not allowed to send raw output")
	}
	if _, err := s.messaging.Message(s.ctx, &messagingpb.MessageRequest{
		Content: []byte(req.Content),
		Format:  format,
		Target: &messagingpb.MessageRequest_UserId{
			UserId: req.ID,
		},
	}); err != nil {
		return err
	}
	return nil
}
