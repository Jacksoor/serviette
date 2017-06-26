package messagingservice

import (
	"fmt"

	"golang.org/x/net/context"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
)

type Service struct {
	ctx       context.Context
	traits    *accountspb.Traits
	messaging messagingpb.MessagingClient
}

func isOutputFormatAllowed(traits *accountspb.Traits, format string) bool {
	for _, allowedFormat := range traits.AllowedOutputFormat {
		if allowedFormat == format {
			return true
		}
	}
	return false
}

func New(ctx context.Context, traits *accountspb.Traits, messaging messagingpb.MessagingClient) *Service {
	return &Service{
		ctx:       ctx,
		traits:    traits,
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

	if !isOutputFormatAllowed(s.traits, format) {
		return fmt.Errorf(`output format "%s" is not allowed`, format)
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

	if !isOutputFormatAllowed(s.traits, format) {
		return fmt.Errorf(`output format "%s" is not allowed`, format)
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
