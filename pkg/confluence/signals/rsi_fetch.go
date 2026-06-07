package signals

import (
	"context"
	"fmt"
)

// MassiveRSIClient wraps a polygon-style RSI fetcher (no caching).
type MassiveRSIClient interface {
	GetRSI(ctx context.Context, ticker string, window int) (float64, error)
}

// PolygonRSIAdapter adapts polygon.Client GetRSI (value + timestamp) to MassiveRSIClient.
type PolygonRSIAdapter struct {
	Fetch func(ctx context.Context, ticker string, window int) (float64, error)
}

// GetRSI fetches the latest RSI value.
func (a PolygonRSIAdapter) GetRSI(ctx context.Context, ticker string, window int) (float64, error) {
	if a.Fetch == nil {
		return 0, fmt.Errorf("RSI fetch not configured")
	}
	return a.Fetch(ctx, ticker, window)
}

// FetchRSI retrieves fresh RSI-14 for a ticker via the given client.
func FetchRSI(ctx context.Context, client MassiveRSIClient, ticker string) (float64, error) {
	if client == nil {
		return 0, fmt.Errorf("RSI client is nil")
	}
	return client.GetRSI(ctx, ticker, 14)
}
