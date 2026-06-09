package confluence

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// SnapshotNeedsBootstrap reports whether a cached snapshot lacks a scored level ladder.
func SnapshotNeedsBootstrap(snap *pkgconfluence.ConfluenceSnapshot) bool {
	if snap == nil {
		return true
	}
	switch snap.OIStatus {
	case pkgconfluence.OIStatusLoading, pkgconfluence.OIStatusError:
		return true
	}
	return !snapshotHasLevels(snap)
}

func snapshotHasLevels(snap *pkgconfluence.ConfluenceSnapshot) bool {
	if snap == nil {
		return false
	}
	return snap.Levels.GammaFlip > 0 ||
		len(snap.Levels.Support) > 0 ||
		len(snap.Levels.Resistance) > 0
}

// BootstrapSnapshot performs a blocking one-shot fetch from Massive and stores a full snapshot.
// Safe to call outside regular trading hours; stale last-session data is acceptable.
func (p *Processor) BootstrapSnapshot(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return nil, err
	}
	if p.client == nil {
		return nil, fmt.Errorf("polygon client not configured")
	}

	mergedCtx, cancel := p.mergeContexts(ctx, p.ctx)
	defer cancel()

	lock := p.bootstrapLock(ticker)
	lock.Lock()
	defer lock.Unlock()

	if snap, ok := p.LatestSnapshot(ticker); ok && !SnapshotNeedsBootstrap(snap) {
		return snap, nil
	}

	snap, err := p.runBootstrapFetch(mergedCtx, ticker)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

func (p *Processor) bootstrapLock(ticker string) *sync.Mutex {
	p.bootstrapMu.Lock()
	defer p.bootstrapMu.Unlock()
	if p.bootstrapLocks == nil {
		p.bootstrapLocks = make(map[string]*sync.Mutex)
	}
	lock, ok := p.bootstrapLocks[ticker]
	if !ok {
		lock = &sync.Mutex{}
		p.bootstrapLocks[ticker] = lock
	}
	return lock
}

func (p *Processor) runBootstrapFetch(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error) {
	now := time.Now().UTC()
	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		return nil, err
	}

	expirations, err := p.client.ResolveExpirations(ctx, ticker, p.settings.UsesDualExpiration(ticker), marketDate)
	if err != nil {
		return nil, fmt.Errorf("resolve expirations: %w", err)
	}
	if len(expirations) == 0 {
		return nil, fmt.Errorf("no expirations found for %s", ticker)
	}

	spot, spotTime, err := FetchLastTrade(ctx, p.client, ticker)
	if err != nil {
		return nil, fmt.Errorf("spot for %s: %w", ticker, err)
	}
	if spot <= 0 {
		return nil, fmt.Errorf("invalid spot price for %s", ticker)
	}

	strikeLow, strikeHigh := StrikeBand(spot)

	var slices []pkgconfluence.OptionSlice
	for _, exp := range expirations {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		expStr := exp.Format("2006-01-02")
		slice, err := FetchOptionSlice(ctx, p.client, p.oiCache, p.settings, ticker, expStr, marketDate, strikeLow, strikeHigh)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if polygon.IsRateLimitError(err) {
				log.Printf("[confluence] bootstrap %s %s: Massive rate limit (429)", ticker, expStr)
			}
			return nil, fmt.Errorf("option slice %s %s: %w", ticker, expStr, err)
		}
		slices = append(slices, *slice)
	}

	rsi, _, rsiErr := p.fetchRSI(ctx, ticker)
	if rsiErr != nil {
		log.Printf("[confluence] bootstrap RSI %s: %v", ticker, rsiErr)
	}

	sectorETF := p.defaultSectorETF()
	rt := p.ensureRuntime(ticker)
	if rt.sectorETF != "" {
		sectorETF = rt.sectorETF
	} else if p.sectors != nil {
		overview, err := p.client.GetTickerOverview(ctx, ticker)
		if err != nil {
			return nil, fmt.Errorf("ticker overview for %s: %w", ticker, err)
		}
		sectorETF = p.sectors.ResolveSectorETF(overview.SICCode, overview.SICDescription)
		if sectorETF == "" {
			sectorETF = p.defaultSectorETF()
		}
		rt.sectorETF = sectorETF
	}

	spySpot, spySpotTime, err := FetchLastTrade(ctx, p.client, "SPY")
	if err != nil {
		return nil, fmt.Errorf("SPY spot: %w", err)
	}
	qqqSpot, qqqSpotTime, err := FetchLastTrade(ctx, p.client, "QQQ")
	if err != nil {
		return nil, fmt.Errorf("QQQ spot: %w", err)
	}
	etfSpot, etfSpotTime, err := FetchLastTrade(ctx, p.client, sectorETF)
	if err != nil {
		return nil, fmt.Errorf("%s spot: %w", sectorETF, err)
	}

	targetDay, err := FetchDayStats(ctx, p.client, p.settings, ticker, now)
	if err != nil {
		return nil, fmt.Errorf("day stats for %s: %w", ticker, err)
	}
	spyDay, err := FetchDayStats(ctx, p.client, p.settings, "SPY", now)
	if err != nil {
		return nil, fmt.Errorf("day stats for SPY: %w", err)
	}
	qqqDay, err := FetchDayStats(ctx, p.client, p.settings, "QQQ", now)
	if err != nil {
		return nil, fmt.Errorf("day stats for QQQ: %w", err)
	}
	etfDay, err := FetchDayStats(ctx, p.client, p.settings, sectorETF, now)
	if err != nil {
		return nil, fmt.Errorf("day stats for %s: %w", sectorETF, err)
	}

	dataAsOf := latestTimestamp(spotTime, spySpotTime, qqqSpotTime, etfSpotTime)
	for _, sl := range slices {
		if sl.GreeksAsOf.After(dataAsOf) {
			dataAsOf = sl.GreeksAsOf
		}
		if sl.OIAsOf.After(dataAsOf) {
			dataAsOf = sl.OIAsOf
		}
	}
	if dataAsOf.IsZero() {
		dataAsOf = now
	}

	marketStatus, err := p.MarketStatusFor(now)
	if err != nil {
		return nil, err
	}

	p.registry.SetOIState(ticker, pkgconfluence.OIStatusReady)

	scoreInput := pkgconfluence.ScoreInput{
		Ticker:        ticker,
		Spot:          spot,
		SpotTime:      spotTime,
		Slices:        slices,
		OIStatus:      pkgconfluence.OIStatusReady,
		MarketStatus:  marketStatus,
		RSI:           rsi,
		SPYOpen:       spyDay.Open,
		SPYSpot:       spySpot,
		QQQOpen:       qqqDay.Open,
		QQQSpot:       qqqSpot,
		TargetOpen:    targetDay.Open,
		ETFOpen:       etfDay.Open,
		ETFSpot:       etfSpot,
		SectorETF:     sectorETF,
		IntradayHigh:  targetDay.High,
		IntradayLow:   targetDay.Low,
		SessionOpen:   targetDay.Open,
		SessionVolume: targetDay.Volume,
		SessionVWAP:   targetDay.VWAP,
		Now:           now,
		DataAsOf:      dataAsOf,
		Settings:      p.settings,
	}
	p.EnrichScoreInput(ctx, ticker, &scoreInput, now)

	snap, err := p.RecomputeSnapshot(ticker, scoreInput, now)
	if err != nil {
		return nil, err
	}
	if SnapshotNeedsBootstrap(snap) {
		return nil, fmt.Errorf("bootstrap for %s produced unscored snapshot", ticker)
	}

	p.tickerMu.Lock()
	if rt := p.tickers[ticker]; rt != nil {
		rt.expirations = expirations
		rt.slices = slices
		rt.greeksSpot = spot
	}
	p.tickerMu.Unlock()

	return snap, nil
}

func (p *Processor) defaultSectorETF() string {
	if p.sectors != nil && p.sectors.DefaultSectorETF != "" {
		return p.sectors.DefaultSectorETF
	}
	return "IWM"
}

func latestTimestamp(times ...time.Time) time.Time {
	var latest time.Time
	for _, ts := range times {
		if ts.After(latest) {
			latest = ts
		}
	}
	return latest.UTC()
}

// bootstrapWatchlistIfClosed prefetches full snapshots for configured watchlist tickers on startup
// when the market is closed so the first client request is warm.
func (p *Processor) bootstrapWatchlistIfClosed() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[confluence] bootstrap watchlist panic: %v", r)
		}
	}()
	if p.client == nil || p.settings == nil || p.isRTHNow() {
		return
	}
	parent := p.ctx
	if parent == nil {
		parent = context.Background()
	}
	for _, sym := range p.settings.PrefetchWatchlist {
		ticker := pkgconfluence.NormalizeTicker(sym)
		if err := pkgconfluence.ValidateTicker(ticker); err != nil {
			continue
		}
		if _, err := p.runBootstrapFetch(parent, ticker); err != nil {
			log.Printf("[confluence] bootstrap prefetch %s: %v", ticker, err)
			continue
		}
	}
}
