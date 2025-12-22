package marketplaceclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"messaging-app/config"

	marketplacepb "gitlab.com/spydotech-group/shared-entity/proto/marketplace/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC connection to the Marketplace service
type Client struct {
	conn   *grpc.ClientConn
	client marketplacepb.MarketplaceServiceClient
}

// New creates a new Marketplace gRPC client using the configured host/port
func New(ctx context.Context, cfg *config.Config) (*Client, error) {
	addr := net.JoinHostPort(cfg.MarketplaceGRPCHost, cfg.MarketplaceGRPCPort)
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to marketplace gRPC at %s: %w", addr, err)
	}

	return &Client{
		conn:   conn,
		client: marketplacepb.NewMarketplaceServiceClient(conn),
	}, nil
}

// Close shuts down the underlying gRPC connection
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// GetClient returns the underlying gRPC client for direct method calls
func (c *Client) GetClient() marketplacepb.MarketplaceServiceClient {
	return c.client
}
