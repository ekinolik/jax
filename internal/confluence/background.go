package confluence

import (
	"context"
	"log"
	"sync"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// testWarmupBlock allows tests to block background warmup for a ticker until the channel is closed.
var testWarmupBlock sync.Map // map[string]<-chan struct{}

func (p *Processor) startBackgroundWarmup(ticker string) {
	if p.ctx == nil {
		return
	}
	ticker = pkgconfluence.NormalizeTicker(ticker)

	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	if rt == nil {
		p.tickerMu.Unlock()
		return
	}
	if rt.warmupRunning {
		p.tickerMu.Unlock()
		return
	}
	rt.warmupRunning = true
	warmupCtx, cancel := context.WithCancel(p.ctx)
	rt.warmupCancel = cancel
	p.tickerMu.Unlock()

	go func() {
		defer func() {
			cancel()
			p.tickerMu.Lock()
			if rt := p.tickers[ticker]; rt != nil {
				rt.warmupRunning = false
				rt.warmupCancel = nil
			}
			p.tickerMu.Unlock()
		}()

		if ch, ok := testWarmupBlock.Load(ticker); ok {
			select {
			case <-ch.(chan struct{}):
			case <-warmupCtx.Done():
				return
			}
		}

		p.warmupTicker(warmupCtx, ticker)
	}()
}

func (p *Processor) stopBackgroundWarmup(ticker string) {
	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	if rt != nil && rt.warmupCancel != nil {
		rt.warmupCancel()
	}
	p.tickerMu.Unlock()
}

// StopBackgroundWarmup cancels in-flight background OI/bootstrap for a ticker.
func (p *Processor) StopBackgroundWarmup(ticker string) {
	p.stopBackgroundWarmup(ticker)
}

// BootstrapInProgress reports whether background OI/bootstrap is still running or OI is loading.
func (p *Processor) BootstrapInProgress(ticker string) bool {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	entry, ok := p.registry.Get(ticker)
	if ok && entry.OIState == pkgconfluence.OIStatusLoading {
		return true
	}
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	rt := p.tickers[ticker]
	return rt != nil && rt.warmupRunning
}

// RecomputeIfReady synchronously rebuilds a scored snapshot when OI is ready and greeks slices exist.
func (p *Processor) RecomputeIfReady(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	entry, ok := p.registry.Get(ticker)
	if !ok || entry.OIState != pkgconfluence.OIStatusReady {
		return nil, false
	}

	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	hasSlices := rt != nil && len(rt.slices) > 0
	p.tickerMu.Unlock()
	if !hasSlices {
		greeksCtx, cancel := withAPITimeout(ctx)
		if err := p.refreshGreeks(greeksCtx, ticker); err != nil {
			cancel()
			return nil, false
		}
		cancel()
	}

	p.recomputeSnapshotNow(ctx, ticker)
	snap, ok := p.LatestSnapshot(ticker)
	if !ok || snap == nil || SnapshotNeedsBootstrap(snap) {
		return nil, false
	}
	return snap, true
}

// recomputeSnapshotNow stores a scored snapshot without requiring active subscribers.
// Background OI warmup uses this so unary handlers that already unsubscribed still get a warm cache.
func (p *Processor) recomputeSnapshotNow(ctx context.Context, ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	entry, ok := p.registry.Get(ticker)
	if !ok || entry.OIState != pkgconfluence.OIStatusReady {
		return
	}

	now := timeNowUTC()
	open, err := p.IsRTH(now)
	if err != nil || !open {
		return
	}

	apiCtx, cancel := p.apiCtx()
	if ctx != nil {
		apiCtx, cancel = p.mergeContexts(apiCtx, ctx)
	}
	defer cancel()

	in, err := p.buildScoreInput(apiCtx, ticker, now)
	if err != nil {
		log.Printf("[confluence] background recompute input %s: %v", ticker, err)
		return
	}
	if len(in.Slices) == 0 {
		return
	}

	snap, err := p.RecomputeSnapshot(ticker, in, now)
	if err != nil {
		log.Printf("[confluence] background recompute %s: %v", ticker, err)
		return
	}
	p.maybePushUpdate(ticker, snap)
}

func (p *Processor) warmupTicker(ctx context.Context, ticker string) {
	if ctx.Err() != nil {
		return
	}

	now := timeNowUTC()
	open, _ := p.IsRTH(now)
	if !open {
		if snap, ok := p.LatestSnapshot(ticker); ok && !SnapshotNeedsBootstrap(snap) {
			return
		}
		lock := p.bootstrapLock(ticker)
		lock.Lock()
		defer lock.Unlock()
		if snap, ok := p.LatestSnapshot(ticker); ok && !SnapshotNeedsBootstrap(snap) {
			return
		}
		snap, err := p.runBootstrapFetch(ctx, ticker)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[confluence] background bootstrap %s: %v", ticker, err)
			p.setOIStateFromCtx(ticker, ctx, pkgconfluence.OIStatusError)
			return
		}
		p.maybePushUpdate(ticker, snap)
		return
	}

	p.ensureOI(ctx, ticker)
}

// timeNowUTC is overridden in tests.
var timeNowUTC = func() time.Time {
	return time.Now().UTC()
}
