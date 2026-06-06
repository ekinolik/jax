package confluence

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ekinolik/jax/internal/stream"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

const (
	apiCallTimeout     = 30 * time.Second
	recomputeDebounce  = 5 * time.Second
	greeksInterval     = 90 * time.Second
	greeksMaxInterval  = 5 * time.Minute
	spotGreeksMovePct  = 0.005
	idleGracePeriod    = 5 * time.Minute
	pushDebouncePeriod = 30 * time.Second
	cleanupInterval    = 30 * time.Second
)

type tickerRuntime struct {
	expirations []time.Time
	sectorETF   string
	slices      []pkgconfluence.OptionSlice
	greeksSpot  float64

	debounceTimer *time.Timer
	greeksTimer   *time.Timer

	watchers      map[int]chan *pkgconfluence.ConfluenceSnapshot
	nextWatcherID int

	lastPushFP string
	lastPushAt time.Time
}

func (p *Processor) apiCtx() (context.Context, context.CancelFunc) {
	parent := p.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, apiCallTimeout)
}

// Start launches background loops for prefetch, idle cleanup, and RTH monitoring.
func (p *Processor) Start(ctx context.Context) error {
	if p.ctx != nil {
		return nil
	}
	p.ctx, p.cancel = context.WithCancel(ctx)

	if p.hub != nil {
		p.hub.OnSpotUpdate(p.onSpotTick)
	}

	p.wg.Add(3)
	go func() {
		defer p.wg.Done()
		p.runPrefetchScheduler()
	}()
	go func() {
		defer p.wg.Done()
		p.runIdleCleanup()
	}()
	go func() {
		defer p.wg.Done()
		p.runRTHMonitor()
	}()
	return nil
}

// Stop shuts down background loops.
func (p *Processor) Stop() {
	if p.cancel == nil {
		return
	}
	p.cancel()
	p.wg.Wait()
	p.cancel = nil
	p.ctx = nil

	p.tickerMu.Lock()
	for ticker, rt := range p.tickers {
		p.stopTimersLocked(rt)
		for _, ch := range rt.watchers {
			close(ch)
		}
		delete(p.tickers, ticker)
	}
	p.tickerMu.Unlock()
}

func (p *Processor) onSpotTick(ticker string, tick stream.SpotTick) {
	ticker = pkgconfluence.NormalizeTicker(ticker)

	p.tickerMu.Lock()
	rt, ok := p.tickers[ticker]
	if ok {
		p.scheduleDebouncedRecomputeLocked(ticker, rt)
		if rt.greeksSpot > 0 && tick.Price > 0 {
			move := math.Abs(tick.Price-rt.greeksSpot) / rt.greeksSpot
			if move > spotGreeksMovePct && p.needsGreeksRefresh(ticker, tick.Price) {
				go p.refreshGreeks(p.ctx, ticker)
			}
		}
		p.tickerMu.Unlock()
		return
	}
	p.tickerMu.Unlock()

	p.scheduleRecomputeForBenchmark(ticker)
}

func (p *Processor) scheduleRecomputeForBenchmark(symbol string) {
	symbol = pkgconfluence.NormalizeTicker(symbol)
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	for ticker, rt := range p.tickers {
		if ticker == symbol || symbol == "SPY" || symbol == "QQQ" || symbol == rt.sectorETF {
			p.scheduleDebouncedRecomputeLocked(ticker, rt)
		}
	}
}

func (p *Processor) scheduleDebouncedRecompute(ticker string) {
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	rt, ok := p.tickers[ticker]
	if !ok {
		return
	}
	p.scheduleDebouncedRecomputeLocked(ticker, rt)
}

func (p *Processor) scheduleDebouncedRecomputeLocked(ticker string, rt *tickerRuntime) {
	if rt.debounceTimer != nil {
		rt.debounceTimer.Stop()
	}
	rt.debounceTimer = time.AfterFunc(recomputeDebounce, func() {
		p.runRecompute(p.ctx, ticker)
	})
}

func (p *Processor) triggerImmediateRecompute(ticker string) {
	go p.runRecompute(p.ctx, ticker)
}

func (p *Processor) startGreeksTimer(ticker string) {
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	rt, ok := p.tickers[ticker]
	if !ok {
		return
	}
	if rt.greeksTimer != nil {
		rt.greeksTimer.Stop()
	}
	rt.greeksTimer = time.AfterFunc(greeksInterval, func() {
		if p.needsGreeksRefresh(ticker, 0) {
			if err := p.refreshGreeks(p.ctx, ticker); err != nil {
				log.Printf("[confluence] greeks refresh %s: %v", ticker, err)
			}
		}
		p.startGreeksTimer(ticker)
	})
}

func (p *Processor) stopGreeksTimer(ticker string) {
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	rt, ok := p.tickers[ticker]
	if !ok {
		return
	}
	if rt.greeksTimer != nil {
		rt.greeksTimer.Stop()
		rt.greeksTimer = nil
	}
}

func (p *Processor) stopTimersLocked(rt *tickerRuntime) {
	if rt.debounceTimer != nil {
		rt.debounceTimer.Stop()
		rt.debounceTimer = nil
	}
	if rt.greeksTimer != nil {
		rt.greeksTimer.Stop()
		rt.greeksTimer = nil
	}
}

func (p *Processor) needsGreeksRefresh(ticker string, spot float64) bool {
	entry, ok := p.registry.Get(ticker)
	if !ok {
		return true
	}
	if entry.LastGreeksAt.IsZero() {
		return true
	}
	elapsed := time.Since(entry.LastGreeksAt)
	if elapsed >= greeksMaxInterval {
		return true
	}
	if elapsed >= greeksInterval {
		return true
	}
	if spot > 0 {
		p.tickerMu.Lock()
		rt := p.tickers[ticker]
		base := 0.0
		if rt != nil {
			base = rt.greeksSpot
		}
		p.tickerMu.Unlock()
		if base > 0 {
			move := math.Abs(spot-base) / base
			if move > spotGreeksMovePct {
				return true
			}
		}
	}
	return false
}

func (p *Processor) runRecompute(ctx context.Context, ticker string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if entry, ok := p.registry.Get(ticker); !ok || entry.SubscriberCount == 0 {
		return
	}

	now := time.Now().UTC()

	open, err := p.IsRTH(now)
	if err != nil {
		log.Printf("[confluence] RTH check %s: %v", ticker, err)
		return
	}
	if !open {
		return
	}

	apiCtx, cancel := p.apiCtx()
	defer cancel()
	if err := p.refreshGreeksIfDue(apiCtx, ticker); err != nil {
		log.Printf("[confluence] greeks %s: %v", ticker, err)
	}

	in, err := p.buildScoreInput(apiCtx, ticker, now)
	if err != nil {
		log.Printf("[confluence] build input %s: %v", ticker, err)
		return
	}

	snap, err := p.RecomputeSnapshot(ticker, in, now)
	if err != nil {
		log.Printf("[confluence] recompute %s: %v", ticker, err)
		return
	}
	p.maybePushUpdate(ticker, snap)
}

func (p *Processor) refreshGreeksIfDue(ctx context.Context, ticker string) error {
	spot, _ := p.spotFor(ctx, ticker)
	if !p.needsGreeksRefresh(ticker, spot) {
		return nil
	}
	return p.refreshGreeks(ctx, ticker)
}

func (p *Processor) refreshGreeks(ctx context.Context, ticker string) error {
	if p.client == nil {
		return fmt.Errorf("polygon client not configured")
	}
	now := time.Now().UTC()
	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		return err
	}

	spot, err := p.spotFor(ctx, ticker)
	if err != nil || spot <= 0 {
		return fmt.Errorf("spot unavailable for %s", ticker)
	}
	strikeLow, strikeHigh := StrikeBand(spot)

	p.tickerMu.Lock()
	rt, ok := p.tickers[ticker]
	if !ok {
		p.tickerMu.Unlock()
		return fmt.Errorf("ticker %s not active", ticker)
	}
	expirations := append([]time.Time(nil), rt.expirations...)
	p.tickerMu.Unlock()

	if len(expirations) == 0 {
		expirations, err = p.client.ResolveExpirations(ctx, ticker, p.settings.UsesDualExpiration(ticker), marketDate)
		if err != nil {
			return err
		}
		p.tickerMu.Lock()
		if rt := p.tickers[ticker]; rt != nil {
			rt.expirations = expirations
		}
		p.tickerMu.Unlock()
	}

	var slices []pkgconfluence.OptionSlice
	for _, exp := range expirations {
		expStr := exp.Format("2006-01-02")
		slice, err := FetchOptionSlice(ctx, p.client, p.oiCache, p.settings, ticker, expStr, marketDate, strikeLow, strikeHigh)
		if err != nil {
			return fmt.Errorf("greeks slice %s %s: %w", ticker, expStr, err)
		}
		slices = append(slices, *slice)
	}

	p.tickerMu.Lock()
	if rt := p.tickers[ticker]; rt != nil {
		rt.slices = slices
		rt.greeksSpot = spot
	}
	p.tickerMu.Unlock()

	p.registry.MarkGreeksRefresh(ticker, now)
	if AllOIReady(p.oiCache, ticker, marketDate, expirations) {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusReady)
	}
	return nil
}

func (p *Processor) buildScoreInput(ctx context.Context, ticker string, now time.Time) (pkgconfluence.ScoreInput, error) {
	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	var slices []pkgconfluence.OptionSlice
	sectorETF := "IWM"
	if p.sectors != nil && p.sectors.DefaultSectorETF != "" {
		sectorETF = p.sectors.DefaultSectorETF
	}
	if rt != nil {
		slices = append([]pkgconfluence.OptionSlice(nil), rt.slices...)
		if rt.sectorETF != "" {
			sectorETF = rt.sectorETF
		}
	}
	p.tickerMu.Unlock()

	spot, spotTime, err := p.spotForWithTime(ctx, ticker)
	if err != nil {
		return pkgconfluence.ScoreInput{}, err
	}

	rsi, _, err := p.client.GetRSI(ctx, ticker, 14)
	if err != nil {
		return pkgconfluence.ScoreInput{}, fmt.Errorf("RSI: %w", err)
	}

	spySpot, _ := p.spotFor(ctx, "SPY")
	qqqSpot, _ := p.spotFor(ctx, "QQQ")
	etfSpot, _ := p.spotFor(ctx, sectorETF)

	targetDay, err := p.dayStatsFor(ctx, ticker, now)
	if err != nil {
		return pkgconfluence.ScoreInput{}, err
	}
	spyDay, err := p.dayStatsFor(ctx, "SPY", now)
	if err != nil {
		return pkgconfluence.ScoreInput{}, err
	}
	qqqDay, err := p.dayStatsFor(ctx, "QQQ", now)
	if err != nil {
		return pkgconfluence.ScoreInput{}, err
	}
	etfDay, err := p.dayStatsFor(ctx, sectorETF, now)
	if err != nil {
		return pkgconfluence.ScoreInput{}, err
	}

	return pkgconfluence.ScoreInput{
		Ticker:       ticker,
		Spot:         spot,
		SpotTime:     spotTime,
		Slices:       slices,
		RSI:          rsi,
		SPYOpen:      spyDay.Open,
		SPYSpot:      spySpot,
		QQQOpen:      qqqDay.Open,
		QQQSpot:      qqqSpot,
		TargetOpen:   targetDay.Open,
		ETFSpot:      etfSpot,
		ETFOpen:      etfDay.Open,
		SectorETF:    sectorETF,
		IntradayHigh: targetDay.High,
		IntradayLow:  targetDay.Low,
		Now:          now,
	}, nil
}

func (p *Processor) spotFor(ctx context.Context, ticker string) (float64, error) {
	price, _, err := p.spotForWithTime(ctx, ticker)
	return price, err
}

func (p *Processor) spotForWithTime(ctx context.Context, ticker string) (float64, time.Time, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if p.hub != nil {
		if tick, ok := p.hub.GetSpot(ticker); ok && tick.Price > 0 {
			return tick.Price, tick.Timestamp, nil
		}
	}
	if p.client != nil {
		return FetchLastTrade(ctx, p.client, ticker)
	}
	return 0, time.Time{}, fmt.Errorf("no spot source for %s", ticker)
}

func (p *Processor) dayStatsFor(ctx context.Context, symbol string, now time.Time) (DayStats, error) {
	symbol = pkgconfluence.NormalizeTicker(symbol)
	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		return DayStats{}, err
	}
	cacheKey := symbol + "_" + marketDate.Format("2006-01-02")

	p.dayStatsMu.Lock()
	if stats, ok := p.dayStats[cacheKey]; ok {
		p.dayStatsMu.Unlock()
		return stats, nil
	}
	p.dayStatsMu.Unlock()

	stats, err := FetchDayStats(ctx, p.client, p.settings, symbol, now)
	if err != nil {
		return DayStats{}, err
	}

	if open, err := p.IsRTH(now); err == nil && open {
		p.dayStatsMu.Lock()
		p.dayStats[cacheKey] = stats
		p.dayStatsMu.Unlock()
	}
	return stats, nil
}

func (p *Processor) maybePushUpdate(ticker string, snap *pkgconfluence.ConfluenceSnapshot) {
	fp := snapshotFingerprint(snap)
	now := time.Now().UTC()

	p.tickerMu.Lock()
	rt, ok := p.tickers[ticker]
	if !ok {
		p.tickerMu.Unlock()
		return
	}
	if fp == rt.lastPushFP && now.Sub(rt.lastPushAt) < pushDebouncePeriod {
		p.tickerMu.Unlock()
		return
	}
	rt.lastPushFP = fp
	rt.lastPushAt = now
	watchers := make([]chan *pkgconfluence.ConfluenceSnapshot, 0, len(rt.watchers))
	for _, ch := range rt.watchers {
		watchers = append(watchers, ch)
	}
	p.tickerMu.Unlock()

	for _, ch := range watchers {
		select {
		case ch <- snap:
		default:
		}
	}
}

func snapshotFingerprint(snap *pkgconfluence.ConfluenceSnapshot) string {
	if snap == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(strconv.FormatFloat(snap.Score, 'f', 1, 64))
	for _, sig := range snap.Signals {
		b.WriteByte('|')
		b.WriteString(sig.Name)
		b.WriteByte(':')
		b.WriteString(string(sig.Status))
	}
	return b.String()
}

func (p *Processor) ensureRuntime(ticker string) *tickerRuntime {
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	rt, ok := p.tickers[ticker]
	if !ok {
		rt = &tickerRuntime{
			watchers: make(map[int]chan *pkgconfluence.ConfluenceSnapshot),
		}
		p.tickers[ticker] = rt
	}
	return rt
}

func (p *Processor) ensureBenchmark(symbol string) error {
	if p.hub == nil {
		return nil
	}
	symbol = pkgconfluence.NormalizeTicker(symbol)
	p.benchMu.Lock()
	defer p.benchMu.Unlock()
	if p.benchmarkRefs[symbol] == 0 {
		if err := p.hub.Subscribe(symbol); err != nil {
			return err
		}
	}
	p.benchmarkRefs[symbol]++
	return nil
}

func (p *Processor) releaseBenchmark(symbol string) {
	if p.hub == nil {
		return
	}
	symbol = pkgconfluence.NormalizeTicker(symbol)
	p.benchMu.Lock()
	defer p.benchMu.Unlock()
	if p.benchmarkRefs[symbol] <= 0 {
		return
	}
	p.benchmarkRefs[symbol]--
	if p.benchmarkRefs[symbol] == 0 {
		_ = p.hub.Unsubscribe(symbol)
	}
}

func (p *Processor) activate(ctx context.Context, ticker string, now time.Time) error {
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

	rt := p.ensureRuntime(ticker)

	if p.hub != nil {
		if err := p.hub.Subscribe(ticker); err != nil {
			p.registry.Unsubscribe(ticker)
			return fmt.Errorf("subscribe stream hub: %w", err)
		}
	}

	for _, bench := range []string{"SPY", "QQQ"} {
		if err := p.ensureBenchmark(bench); err != nil {
			p.deactivateTicker(ticker)
			return fmt.Errorf("subscribe benchmark %s: %w", bench, err)
		}
	}

	defaultETF := "IWM"
	if p.sectors != nil && p.sectors.DefaultSectorETF != "" {
		defaultETF = p.sectors.DefaultSectorETF
	}
	if err := p.resolveSectorETF(ctx, ticker, rt); err != nil {
		log.Printf("[confluence] sector resolve %s: %v (using default)", ticker, err)
		rt.sectorETF = defaultETF
	}
	if rt.sectorETF == "" {
		rt.sectorETF = defaultETF
	}
	if err := p.ensureBenchmark(rt.sectorETF); err != nil {
		p.deactivateTicker(ticker)
		return fmt.Errorf("subscribe sector ETF %s: %w", rt.sectorETF, err)
	}

	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:       ticker,
		OIStatus:     entry.OIState,
		MarketStatus: status,
		UpdatedAt:    now.UTC(),
		SectorETF:    rt.sectorETF,
	}
	if p.hub != nil {
		if tick, ok := p.hub.GetSpot(ticker); ok {
			snap.Spot = tick.Price
			snap.SpotTime = tick.Timestamp
		}
	}
	p.SetSnapshot(ticker, snap)

	oiCtx := p.ctx
	if oiCtx == nil {
		oiCtx = context.Background()
	}
	go p.ensureOI(oiCtx, ticker)

	if open, _ := p.IsRTH(now); open {
		p.startGreeksTimer(ticker)
		p.triggerImmediateRecompute(ticker)
	}
	return nil
}

func (p *Processor) resolveSectorETF(ctx context.Context, ticker string, rt *tickerRuntime) error {
	if rt.sectorETF != "" {
		return nil
	}
	if p.client == nil || p.sectors == nil {
		return fmt.Errorf("sector config unavailable")
	}
	overview, err := p.client.GetTickerOverview(ctx, ticker)
	if err != nil {
		return err
	}
	rt.sectorETF = p.sectors.ResolveSectorETF(overview.SICCode, overview.SICDescription)
	return nil
}

func (p *Processor) ensureOI(ctx context.Context, ticker string) {
	if p.client == nil {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusError)
		return
	}

	now := time.Now().UTC()
	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusError)
		return
	}

	p.tickerMu.Lock()
	rt := p.tickers[ticker]
	var expirations []time.Time
	if rt != nil {
		expirations = append([]time.Time(nil), rt.expirations...)
	}
	p.tickerMu.Unlock()

	if len(expirations) == 0 {
		expirations, err = p.client.ResolveExpirations(ctx, ticker, p.settings.UsesDualExpiration(ticker), marketDate)
		if err != nil {
			log.Printf("[confluence] resolve expirations %s: %v", ticker, err)
			p.registry.SetOIState(ticker, pkgconfluence.OIStatusError)
			return
		}
		p.tickerMu.Lock()
		if rt := p.tickers[ticker]; rt != nil {
			rt.expirations = expirations
		}
		p.tickerMu.Unlock()
	}

	if len(expirations) == 0 {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusError)
		return
	}

	if AllOIReady(p.oiCache, ticker, marketDate, expirations) {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusReady)
		if open, _ := p.IsRTH(now); open {
			p.triggerImmediateRecompute(ticker)
		}
		return
	}

	p.registry.SetOIState(ticker, pkgconfluence.OIStatusLoading)

	spot, err := p.spotFor(ctx, ticker)
	if err != nil || spot <= 0 {
		p.registry.SetOIState(ticker, pkgconfluence.OIStatusLoading)
		return
	}
	strikeLow, strikeHigh := StrikeBand(spot)

	for _, exp := range expirations {
		expStr := exp.Format("2006-01-02")
		if p.oiCache != nil && p.oiCache.HasOI(ticker, marketDate, expStr) {
			continue
		}
		if _, err := FetchOISlice(ctx, p.client, p.oiCache, p.settings, ticker, expStr, marketDate, strikeLow, strikeHigh); err != nil {
			log.Printf("[confluence] OI fetch %s %s: %v", ticker, expStr, err)
			p.registry.SetOIState(ticker, pkgconfluence.OIStatusError)
			return
		}
	}

	p.registry.SetOIState(ticker, pkgconfluence.OIStatusReady)
	if open, _ := p.IsRTH(now); open {
		if err := p.refreshGreeks(ctx, ticker); err != nil {
			log.Printf("[confluence] greeks after OI %s: %v", ticker, err)
		}
		p.triggerImmediateRecompute(ticker)
	}
}

func (p *Processor) deactivateTicker(ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)

	p.tickerMu.Lock()
	rt, ok := p.tickers[ticker]
	if ok {
		p.stopTimersLocked(rt)
		sectorETF := rt.sectorETF
		for _, ch := range rt.watchers {
			close(ch)
		}
		delete(p.tickers, ticker)
		p.tickerMu.Unlock()
		if sectorETF != "" {
			p.releaseBenchmark(sectorETF)
		}
	} else {
		p.tickerMu.Unlock()
	}

	if p.hub != nil {
		_ = p.hub.Unsubscribe(ticker)
	}
	p.registry.Remove(ticker)
	if !p.registry.HasEntries() {
		p.releaseBenchmark("SPY")
		p.releaseBenchmark("QQQ")
	}

	p.mu.Lock()
	delete(p.snapshots, ticker)
	p.mu.Unlock()
}

func (p *Processor) runIdleCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			for _, sym := range p.registry.IdleTickers(idleGracePeriod) {
				p.deactivateTicker(sym)
			}
		}
	}
}

func (p *Processor) runRTHMonitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	wasRTH := p.isRTHNow()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			rth := p.isRTHNow()
			if rth && !wasRTH {
				p.onRTHOpen()
			} else if !rth && wasRTH {
				p.onRTHClose()
			}
			wasRTH = rth
		}
	}
}

func (p *Processor) isRTHNow() bool {
	open, err := p.IsRTH(time.Now().UTC())
	return err == nil && open
}

func (p *Processor) onRTHOpen() {
	for _, ticker := range p.registry.ActiveTickers() {
		p.startGreeksTimer(ticker)
		p.triggerImmediateRecompute(ticker)
	}
}

func (p *Processor) onRTHClose() {
	p.tickerMu.Lock()
	for _, rt := range p.tickers {
		if rt.greeksTimer != nil {
			rt.greeksTimer.Stop()
			rt.greeksTimer = nil
		}
	}
	p.tickerMu.Unlock()
}

func (p *Processor) runPrefetchScheduler() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case now := <-ticker.C:
			if !p.shouldPrefetchNow(now) {
				continue
			}
			marketDate, err := MarketCalendarDate(p.settings, now)
			if err != nil {
				continue
			}
			dateKey := marketDate.Format("2006-01-02")
			p.prefetchMu.Lock()
			if p.lastPrefetchDate == dateKey {
				p.prefetchMu.Unlock()
				continue
			}
			p.lastPrefetchDate = dateKey
			p.prefetchMu.Unlock()

			if p.oiCache != nil {
				_ = p.oiCache.PurgeBefore(marketDate)
			}
			go p.prefetchWatchlist(p.ctx)
		}
	}
}

func (p *Processor) shouldPrefetchNow(now time.Time) bool {
	if p.settings == nil {
		return false
	}
	loc, err := time.LoadLocation(p.settings.MarketHours.Timezone)
	if err != nil {
		return false
	}
	local := now.In(loc)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	prefetchTime := p.settings.OIPrefetchTime
	if prefetchTime == "" {
		prefetchTime = "08:00"
	}
	return local.Format("15:04") == prefetchTime
}

func (p *Processor) prefetchWatchlist(ctx context.Context) {
	if p.client == nil || p.settings == nil {
		return
	}
	now := time.Now().UTC()
	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		log.Printf("[confluence] prefetch market date: %v", err)
		return
	}

	for _, sym := range p.settings.PrefetchWatchlist {
		ticker := pkgconfluence.NormalizeTicker(sym)
		if err := pkgconfluence.ValidateTicker(ticker); err != nil {
			continue
		}
		if err := p.prefetchTickerOI(ctx, ticker, marketDate); err != nil {
			log.Printf("[confluence] prefetch OI %s: %v", ticker, err)
		} else {
			log.Printf("[confluence] prefetched OI for %s", ticker)
		}
	}
}

func (p *Processor) prefetchTickerOI(ctx context.Context, ticker string, marketDate time.Time) error {
	expirations, err := p.client.ResolveExpirations(ctx, ticker, p.settings.UsesDualExpiration(ticker), marketDate)
	if err != nil {
		return err
	}
	spot, err := p.spotFor(ctx, ticker)
	if err != nil || spot <= 0 {
		spot = 100
	}
	strikeLow, strikeHigh := StrikeBand(spot)
	for _, exp := range expirations {
		expStr := exp.Format("2006-01-02")
		if p.oiCache != nil && p.oiCache.HasOI(ticker, marketDate, expStr) {
			continue
		}
		if _, err := FetchOISlice(ctx, p.client, p.oiCache, p.settings, ticker, expStr, marketDate, strikeLow, strikeHigh); err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) removeWatcher(ticker string, id int) {
	p.tickerMu.Lock()
	defer p.tickerMu.Unlock()
	if rt, ok := p.tickers[ticker]; ok {
		if ch, ok := rt.watchers[id]; ok {
			close(ch)
			delete(rt.watchers, id)
		}
	}
}
