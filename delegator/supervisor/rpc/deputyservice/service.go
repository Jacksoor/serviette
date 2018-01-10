package deputyservice

import (
	"golang.org/x/net/context"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"

	adminpb "github.com/porpoises/kobun4/executor/adminservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	ctx          context.Context
	context      *scriptspb.Context
	adminClient  adminpb.AdminClient
}

func New(ctx context.Context, context *scriptspb.Context, adminClient adminpb.AdminClient) *Service {
	return &Service{
		ctx:         ctx,
		context:     context,
		adminClient: adminClient,
	}
}

func (s *Service) DeleteInputMessage(req *struct {}, resp *srpc.Response) error {
	if _, err := s.adminClient.DeleteMessage(s.ctx, &adminpb.DeleteMessageRequest{
		Context:   s.context,
		ChannelId: s.context.ChannelId,
		MessageId: s.context.InputMessageId,
	}); err != nil {
		return err
	}

	return nil
}
