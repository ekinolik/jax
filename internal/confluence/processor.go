package confluence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	"github.com/ekinolik/jax/internal/stream"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// Processor coordinates confluence background work during regular trading hours.
type Processor struct {
	settings *pkgconfluence.Settings
	registry *Registry
	oiCache  *OICache
	client   *polygon.Client
	hub      *stream.Hub

	mu        sync.RWMutex
	snapshots map[string]*pkgconfluence.ConfluenceSnapshot
}

// NewProcessor creates a confluence processor skeleton wired to settings and dependencies.
func NewProcessor(
	settings *pkgconfluence.Settings,
	registry *Registry,
	oiCache *OICache,
	client *polygon.Client,
	hub *stream.Hub,
) *Processor {
	return &Processor{
		settings:  settings,
		registry:  registry,
		oiCache:   oiCache,
		client:    client,
		hub:       hub,
		snapshots: make(map[string]*pkgconfluence.ConfluenceSnapshot),
	}
}

// IsRTH reports whether confluence processing should run at the given time.
func (p *Processor) IsRTH(now time.Time) (bool, error) {
	if p.settings == nil {
		return false, fmt.Errorf("confluence settings not loaded")
	}
	return p.settings.IsRTH(now)
}

// LatestSnapshot returns the in-memory snapshot for a ticker, if present.
func (p *Processor) LatestSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	p.mu.RLock()
	defer p.mu.RUnlock()
	snap, ok := p.snapshots[ticker]
	return snap, ok
}

// SetSnapshot stores the latest snapshot for a ticker.
func (p *Processor) SetSnapshot(ticker string, snap *pkgconfluence.ConfluenceSnapshot) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots[ticker] = snap
}

// MarketStatusFor returns open/closed status based on configured RTH.
func (p *Processor) MarketStatusFor(now time.Time) (pkgconfluence.MarketStatus, error) {
	open, err := p.IsRTH(now)
	if err != nil {
		return "", err
	}
	if open {
		return pkgconfluence.MarketStatusOpen, nil
	}
	return pkgconfluence.MarketStatusClosed, nil
}

// OnSubscribe registers a ticker for processing when within RTH.
// Full recompute/scoring is implemented in later phases.
func (p *Processor) OnSubscribe(ctx context.Context, ticker string, now time.Time) error {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return err
	}

	status, err := p.MarketStatusFor(now)
	if err != nil {
		return err
	}

	entry, err := p.registry.Subscribe(ticker)
	if err != nil {
		return err
	}

	if p.hub != nil {
		if err := p.hub.Subscribe(ticker); err != nil {
			p.registry.Unsubscribe(ticker)
			return fmt.Errorf("subscribe stream hub: %w", err)
		}
	}

	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:       ticker,
		OIStatus:     entry.OIState,
		MarketStatus: status,
		UpdatedAt:    now.UTC(),
	}
	if p.hub != nil {
		if tick, ok := p.hub.GetSpot(ticker); ok {
			snap.Spot = tick.Price
			snap.SpotTime = tick.Timestamp
		}
	}

	p.SetSnapshot(ticker, snap)
	_ = ctx
	return nil
}

// OnUnsubscribe decrements registry ref-count for a ticker.
func (p *Processor) OnUnsubscribe(ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	p.registry.Unsubscribe(ticker)

	if p.hub != nil {
		if err := p.hub.Unsubscribe(ticker); err != nil {
			// Logged by caller in later phases; keep registry/hub best-effort in Phase 0.
			_ = err
		}
	}

	entry, ok := p.registry.Get(ticker)
	if ok && entry.SubscriberCount == 0 {
		p.mu.Lock()
		delete(p.snapshots, ticker)
		p.mu.Unlock()
	}
}
