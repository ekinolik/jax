package confluence

import (
	"context"
	"fmt"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
)

func (p *Processor) recomputeDebounce() time.Duration {
	if p.settings != nil && p.settings.Tuning.RecomputeDebounceSec > 0 {
		return time.Duration(p.settings.Tuning.RecomputeDebounceSec) * time.Second
	}
	return 5 * time.Second
}

func (p *Processor) greeksInterval() time.Duration {
	if p.settings != nil && p.settings.Tuning.GreeksIntervalSec > 0 {
		return time.Duration(p.settings.Tuning.GreeksIntervalSec) * time.Second
	}
	return 90 * time.Second
}

func (p *Processor) maxRSICallsPerMinute() int {
	if p.settings != nil && p.settings.Tuning.MaxRSICallsPerMinute > 0 {
		return p.settings.Tuning.MaxRSICallsPerMinute
	}
	return 12
}

func (p *Processor) applyRetryConfig(client *polygon.Client) {
	if client == nil || p.settings == nil {
		return
	}
	client.SetRetryConfig(polygon.RetryConfig{
		MaxRetries:  p.settings.APIRetry.MaxRetries,
		BaseDelayMs: p.settings.APIRetry.BaseDelayMs,
	})
}

func (p *Processor) fetchRSI(ctx context.Context, ticker string) (float64, time.Time, error) {
	if p.client == nil {
		return 0, time.Time{}, fmt.Errorf("polygon client not configured")
	}
	if err := p.acquireRSISlot(ctx); err != nil {
		return 0, time.Time{}, err
	}
	return p.client.GetRSI(ctx, ticker, 14, "minute")
}

func (p *Processor) acquireRSISlot(ctx context.Context) error {
	limit := p.maxRSICallsPerMinute()
	window := time.Minute

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		now := time.Now().UTC()
		p.rsiCallsMu.Lock()
		cutoff := now.Add(-window)
		filtered := p.rsiCallTimes[:0]
		for _, ts := range p.rsiCallTimes {
			if ts.After(cutoff) {
				filtered = append(filtered, ts)
			}
		}
		p.rsiCallTimes = filtered
		if len(p.rsiCallTimes) < limit {
			p.rsiCallTimes = append(p.rsiCallTimes, now)
			p.rsiCallsMu.Unlock()
			return nil
		}
		oldest := p.rsiCallTimes[0]
		p.rsiCallsMu.Unlock()

		wait := window - now.Sub(oldest)
		if wait <= 0 {
			wait = 50 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
