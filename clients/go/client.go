package jax

import (
	"context"
	"fmt"

	jaxv1 "github.com/ekinolik/jax/api/proto/option/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client represents a JAX service client
type Client struct {
	conn   *grpc.ClientConn
	client jaxv1.OptionServiceClient
}

// NewClient creates a new JAX service client
func NewClient(address string) (*Client, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to JAX service: %v", err)
	}

	return &Client{
		conn:   conn,
		client: jaxv1.NewOptionServiceClient(conn),
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetDex retrieves delta exposure calculations for the given parameters
func (c *Client) GetDex(ctx context.Context, underlyingAsset string, startStrike, endStrike *float64) (*jaxv1.GetDexResponse, error) {
	req := &jaxv1.GetDexRequest{
		UnderlyingAsset:  underlyingAsset,
		StartStrikePrice: startStrike,
		EndStrikePrice:   endStrike,
	}

	return c.client.GetDex(ctx, req)
}
