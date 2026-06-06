package confluence

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	"github.com/ekinolik/jax/internal/stream"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
)

// Processor coordinates confluence background work during regular trading hours.
type Processor struct {
	settings *pkgconfluence.Settings
	sectors  *pkgconfluence.SICSectors
	registry *Registry
	oiCache  *OICache
	client   *polygon.Client
	hub      *stream.Hub

	mu        sync.RWMutex
	snapshots map[string]*pkgconfluence.ConfluenceSnapshot

	tickerMu sync.Mutex
	tickers  map[string]*tickerRuntime

	benchMu       sync.Mutex
	benchmarkRefs map[string]int

	dayStatsMu sync.Mutex
	dayStats   map[string]DayStats

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	prefetchMu       sync.Mutex
	lastPrefetchDate string

	bootstrapMu    sync.Mutex
	bootstrapLocks map[string]*sync.Mutex

	rsiCallsMu   sync.Mutex
	rsiCallTimes []time.Time
}

// NewProcessor creates a confluence processor wired to settings and dependencies.
func NewProcessor(
	settings *pkgconfluence.Settings,
	sectors *pkgconfluence.SICSectors,
	registry *Registry,
	oiCache *OICache,
	client *polygon.Client,
	hub *stream.Hub,
) *Processor {
	return &Processor{
		settings:      settings,
		sectors:       sectors,
		registry:      registry,
		oiCache:       oiCache,
		client:        client,
		hub:           hub,
		snapshots:     make(map[string]*pkgconfluence.ConfluenceSnapshot),
		tickers:       make(map[string]*tickerRuntime),
		benchmarkRefs: make(map[string]int),
		dayStats:      make(map[string]DayStats),
	}
}

// IsRTH reports whether confluence processing should run at the given time.
func (p *Processor) IsRTH(now time.Time) (bool, error) {
	if p.settings == nil {
		return false, fmt.Errorf("confluence settings not loaded")
	}
	return p.settings.IsRTH(now)
}

// GetSnapshot returns the latest cached snapshot for a ticker.
func (p *Processor) GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	return p.LatestSnapshot(ticker)
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

// Watch registers a ticker for live updates. The returned channel receives snapshots when
// score or signal status changes (duplicate pushes debounced to 30s). Call unsubscribe when done.
func (p *Processor) Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	now := time.Now().UTC()
	if err := p.activate(ctx, ticker, now); err != nil {
		return nil, nil, err
	}

	if snap, ok := p.LatestSnapshot(ticker); !ok || SnapshotNeedsBootstrap(snap) {
		bootstrapCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
		if _, err := p.BootstrapSnapshot(bootstrapCtx, ticker); err != nil {
			log.Printf("[confluence] bootstrap watch %s: %v", ticker, err)
		}
		cancel()
	}

	ch := make(chan *pkgconfluence.ConfluenceSnapshot, 4)
	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	id := rt.nextWatcherID
	rt.nextWatcherID++
	rt.watchers[id] = ch
	p.tickerMu.Unlock()

	if snap, ok := p.LatestSnapshot(ticker); ok && !SnapshotNeedsBootstrap(snap) {
		select {
		case ch <- snap:
		default:
		}
	}

	unsub := func() {
		p.removeWatcher(ticker, id)
		p.OnUnsubscribe(ticker)
	}
	return ch, unsub, nil
}

// OnSubscribe registers a ticker for processing without a push channel.
func (p *Processor) OnSubscribe(ctx context.Context, ticker string, now time.Time) error {
	return p.activate(ctx, ticker, now)
}

// RecomputeSnapshot builds a scored snapshot from available inputs and stores it.
func (p *Processor) RecomputeSnapshot(ticker string, in pkgconfluence.ScoreInput, now time.Time) (snap *pkgconfluence.ConfluenceSnapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("snapshot compute panic for %s: %v", ticker, r)
		}
	}()

	ticker = pkgconfluence.NormalizeTicker(ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return nil, err
	}

	status, err := p.MarketStatusFor(now)
	if err != nil {
		return nil, err
	}

	if in.Spot <= 0 && p.hub != nil {
		if tick, ok := p.hub.GetSpot(ticker); ok {
			in.Spot = tick.Price
			in.SpotTime = tick.Timestamp
		}
	}

	oiStatus := pkgconfluence.OIStatusLoading
	if entry, ok := p.registry.Get(ticker); ok {
		oiStatus = entry.OIState
	}
	if len(in.Slices) > 0 {
		oiStatus = pkgconfluence.OIStatusReady
	}

	in.Ticker = ticker
	in.OIStatus = oiStatus
	in.MarketStatus = status
	in.Now = now

	snap = new(pkgconfluence.ConfluenceSnapshot)
	*snap = signals.BuildSnapshot(in)
	if snap.DataAsOf.IsZero() {
		snap.DataAsOf = dataAsOfFromScoreInput(in)
	}
	p.SetSnapshot(ticker, snap)
	return snap, nil
}

func dataAsOfFromScoreInput(in pkgconfluence.ScoreInput) time.Time {
	if !in.DataAsOf.IsZero() {
		return in.DataAsOf.UTC()
	}
	latest := in.SpotTime
	for _, sl := range in.Slices {
		if sl.GreeksAsOf.After(latest) {
			latest = sl.GreeksAsOf
		}
		if sl.OIAsOf.After(latest) {
			latest = sl.OIAsOf
		}
	}
	if latest.IsZero() {
		latest = in.Now
	}
	return latest.UTC()
}

// ApplyClientRetryConfig wires Massive REST retry settings from confluence config.
func (p *Processor) ApplyClientRetryConfig(client *polygon.Client) {
	p.applyRetryConfig(client)
}

// OnUnsubscribe decrements registry ref-count for a ticker.
// Hub and timers are released after idleGracePeriod via deactivateTicker.
func (p *Processor) OnUnsubscribe(ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	p.registry.Unsubscribe(ticker)

	entry, ok := p.registry.Get(ticker)
	if ok && entry.SubscriberCount == 0 {
		p.stopGreeksTimer(ticker)
		p.tickerMu.Lock()
		if rt, ok := p.tickers[ticker]; ok {
			p.stopTimersLocked(rt)
		}
		p.tickerMu.Unlock()
	}
}
