package core

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

type gatewayService struct {
	gatewayv1.UnimplementedGatewayServiceServer
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func registerGatewayService(srv *grpc.Server, pool *pgxpool.Pool, logger *slog.Logger) {
	gatewayv1.RegisterGatewayServiceServer(srv, &gatewayService{
		pool:   pool,
		logger: logger,
	})
}

func (s *gatewayService) GetConfig(ctx context.Context, req *gatewayv1.ConfigRequest) (*gatewayv1.GatewayConfig, error) {
	s.logger.Info("gateway requesting config", "token", req.GetGatewayToken()[:8]+"...")

	// Return a minimal config so the gateway can start.
	return &gatewayv1.GatewayConfig{
		GatewayId:   "gw-default",
		NetworkName: "default",
		ListenPort:  51820,
		Addresses:   []string{"10.10.0.1/16"},
	}, nil
}

func (s *gatewayService) Sync(stream grpc.BidiStreamingServer[gatewayv1.GatewayEvent, gatewayv1.CoreEvent]) error {
	s.logger.Info("gateway sync stream opened")

	// Keep the stream open — receive events from gateway.
	for {
		event, err := stream.Recv()
		if err != nil {
			s.logger.Info("gateway sync stream closed", "error", err)
			return err
		}

		switch {
		case event.GetStats() != nil:
			s.logger.Debug("received peer stats", "peers", len(event.GetStats().GetPeers()))
		case event.GetS2SHealth() != nil:
			s.logger.Debug("received s2s health report")
		case event.GetStatus() != nil:
			s.logger.Debug("received gateway status",
				"active_peers", event.GetStatus().GetActivePeers())
		}
	}
}

func (s *gatewayService) Heartbeat(ctx context.Context, req *gatewayv1.HeartbeatRequest) (*emptypb.Empty, error) {
	s.logger.Debug("gateway heartbeat", "gateway_id", req.GetGatewayId())
	return &emptypb.Empty{}, nil
}
