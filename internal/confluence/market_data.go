package confluence

import (
	"context"
	"log"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

const (
	shortInterestCacheTTL = 14 * 24 * time.Hour
	floatSharesCacheTTL   = 7 * 24 * time.Hour
)

type cachedShortInterest struct {
	data      *shortInterestCacheEntry
	fetchedAt time.Time
}

type shortInterestCacheEntry struct {
	ShortInterestPct float64
	DaysToCover      float64
	AvgDailyVolume   float64
	FloatShares      float64
}

type tickerFloatCache struct {
	floatShares float64
	fetchedAt   time.Time
}

// EnrichScoreInput attaches v2 market data (ADR, squeeze, RSI daily) with cost-aware caching.
func (p *Processor) EnrichScoreInput(ctx context.Context, ticker string, in *pkgconfluence.ScoreInput, now time.Time) {
	if p.client == nil || in == nil {
		return
	}
	in.Settings = p.settings

	marketDate, err := MarketCalendarDate(p.settings, now)
	if err != nil {
		return
	}
	sessionKey := ticker + "_" + marketDate.Format("2006-01-02")

	// Daily bars + ADR (once per session day)
	p.dailyBarsMu.Lock()
	bars, barsOK := p.dailyBars[sessionKey]
	p.dailyBarsMu.Unlock()
	if !barsOK {
		fetched, fetchErr := FetchDailyBars(ctx, p.client, p.settings, ticker, now)
		if fetchErr != nil {
			log.Printf("[confluence] daily bars %s: %v", ticker, fetchErr)
		} else {
			bars = fetched
			barsOK = true
			p.dailyBarsMu.Lock()
			p.dailyBars[sessionKey] = bars
			p.dailyBarsMu.Unlock()
		}
	}
	if barsOK && len(bars) > 0 {
		cfg := pkgconfluence.DefaultScoringConfig()
		if p.settings != nil {
			cfg = p.settings.Scoring
		}
		in.ADR = pkgconfluence.ComputeADR(bars, cfg)
		in.New20DayHigh, in.New52WeekHigh = pkgconfluence.BreakoutFlags(bars, in.Spot)
		if in.TargetOpen > 0 && in.Spot > 0 {
			in.GapUpPct = (in.Spot - in.TargetOpen) / in.TargetOpen
		}
	}

	// RSI daily (once per session day)
	p.rsiDailyMu.Lock()
	rsiDaily, rsiDailyOK := p.rsiDaily[sessionKey]
	p.rsiDailyMu.Unlock()
	if !rsiDailyOK {
		if val, _, fetchErr := p.fetchRSIDaily(ctx, ticker); fetchErr != nil {
			log.Printf("[confluence] RSI daily %s: %v", ticker, fetchErr)
		} else if val > 0 {
			rsiDaily = val
			rsiDailyOK = true
			p.rsiDailyMu.Lock()
			p.rsiDaily[sessionKey] = val
			p.rsiDailyMu.Unlock()
		}
	}
	if rsiDailyOK {
		in.RSIDaily = rsiDaily
	}

	// Float shares (7d cache)
	floatShares := p.floatSharesFor(ctx, ticker)

	// Short interest (14d cache)
	si := p.shortInterestFor(ctx, ticker, floatShares)
	if si != nil {
		in.ShortInterestPct = si.ShortInterestPct
		in.DaysToCover = si.DaysToCover
		if si.FloatShares > 0 {
			in.FloatShares = si.FloatShares
		}
	}
	if in.FloatShares <= 0 && floatShares > 0 {
		in.FloatShares = floatShares
	}

	// Short volume ratio (daily cache)
	p.shortVolMu.Lock()
	svRatio, svOK := p.shortVolRatio[sessionKey]
	p.shortVolMu.Unlock()
	if !svOK {
		if data, fetchErr := p.client.GetShortVolumeRatio(ctx, ticker); fetchErr != nil {
			log.Printf("[confluence] short volume %s: %v", ticker, fetchErr)
		} else if data != nil {
			svRatio = data.ShortVolumeRatio
			svOK = true
			p.shortVolMu.Lock()
			p.shortVolRatio[sessionKey] = svRatio
			p.shortVolMu.Unlock()
		}
	}
	if svOK {
		in.ShortVolumeRatio = svRatio
	}

	// Relative volume from session volume vs avg daily
	if in.SessionVolume > 0 {
		avgVol := 0.0
		if si != nil && si.AvgDailyVolume > 0 {
			avgVol = si.AvgDailyVolume
		} else if barsOK {
			avgVol = pkgconfluence.AvgDailyVolume(bars)
		}
		if avgVol > 0 {
			in.RelativeVolume = in.SessionVolume / avgVol
		}
	}
}

func (p *Processor) floatSharesFor(ctx context.Context, ticker string) float64 {
	p.floatMu.Lock()
	cached, ok := p.floatCache[ticker]
	p.floatMu.Unlock()
	if ok && time.Since(cached.fetchedAt) < floatSharesCacheTTL {
		return cached.floatShares
	}
	overview, err := p.client.GetTickerOverview(ctx, ticker)
	if err != nil {
		log.Printf("[confluence] float shares %s: %v", ticker, err)
		return 0
	}
	p.floatMu.Lock()
	p.floatCache[ticker] = tickerFloatCache{floatShares: overview.FloatShares, fetchedAt: time.Now().UTC()}
	p.floatMu.Unlock()
	return overview.FloatShares
}

func (p *Processor) shortInterestFor(ctx context.Context, ticker string, floatShares float64) *shortInterestCacheEntry {
	p.siMu.Lock()
	cached, ok := p.shortInterest[ticker]
	p.siMu.Unlock()
	if ok && time.Since(cached.fetchedAt) < shortInterestCacheTTL {
		return cached.data
	}
	data, err := p.client.GetShortInterest(ctx, ticker, floatShares)
	if err != nil {
		log.Printf("[confluence] short interest %s: %v", ticker, err)
		return nil
	}
	entry := &shortInterestCacheEntry{
		ShortInterestPct: data.ShortInterestPct,
		DaysToCover:      data.DaysToCover,
		AvgDailyVolume:   float64(data.AvgDailyVolume),
		FloatShares:      floatShares,
	}
	p.siMu.Lock()
	p.shortInterest[ticker] = cachedShortInterest{data: entry, fetchedAt: time.Now().UTC()}
	p.siMu.Unlock()
	return entry
}

func (p *Processor) fetchRSIDaily(ctx context.Context, ticker string) (float64, time.Time, error) {
	if err := p.acquireRSISlot(ctx); err != nil {
		return 0, time.Time{}, err
	}
	return p.client.GetRSI(ctx, ticker, 14, "day")
}

func (p *Processor) clearSessionCaches() {
	p.dailyBarsMu.Lock()
	p.dailyBars = make(map[string][]pkgconfluence.DailyBar)
	p.dailyBarsMu.Unlock()

	p.rsiDailyMu.Lock()
	p.rsiDaily = make(map[string]float64)
	p.rsiDailyMu.Unlock()

	p.shortVolMu.Lock()
	p.shortVolRatio = make(map[string]float64)
	p.shortVolMu.Unlock()

	p.dayStatsMu.Lock()
	p.dayStats = make(map[string]DayStats)
	p.dayStatsMu.Unlock()
}
