package confluence

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/massive-com/client-go/v2/rest/models"
)

const defaultStrikeBand = 0.15

// DayStats holds session open/high/low for a symbol.
type DayStats struct {
	Open float64
	High float64
	Low  float64
}

// StrikeBand returns the default ±15% strike window around spot.
func StrikeBand(spot float64) (float64, float64) {
	return strikeBand(spot, defaultStrikeBand)
}

func strikeBand(spot, band float64) (float64, float64) {
	if band <= 0 {
		band = defaultStrikeBand
	}
	low := spot * (1 - band)
	high := spot * (1 + band)
	if low < 0 {
		low = 0
	}
	return math.Floor(low*100) / 100, math.Ceil(high*100) / 100
}

// MarketCalendarDate returns the market timezone calendar date for now.
func MarketCalendarDate(settings *pkgconfluence.Settings, now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("load market timezone: %w", err)
	}
	return pkgconfluence.DateOnly(now.In(loc)), nil
}

// FetchLastTrade returns the latest trade price and timestamp for a ticker.
func FetchLastTrade(ctx context.Context, client *polygon.Client, ticker string) (float64, time.Time, error) {
	params := &models.GetLastTradeParams{Ticker: ticker}
	res, err := client.GetLastTrade(ctx, params)
	if err != nil {
		return 0, time.Time{}, err
	}
	ts := time.Time(res.Results.ParticipantTimestamp)
	if ts.IsZero() {
		ts = time.Time(res.Results.Timestamp)
	}
	if ts.IsZero() {
		ts = time.Time(res.Results.TRFTimestamp)
	}
	return res.Results.Price, ts, nil
}

// FetchOptionSlice loads or fetches OI and greeks for one expiration.
func FetchOptionSlice(
	ctx context.Context,
	client *polygon.Client,
	oiCache *OICache,
	settings *pkgconfluence.Settings,
	ticker, expiration string,
	marketDate time.Time,
	strikeLow, strikeHigh float64,
) (*pkgconfluence.OptionSlice, error) {
	monthlyWeight := settings.MonthlyExpiryWeight
	weeklyWeight := settings.WeeklyExpiryWeight

	var cachedOI *pkgconfluence.OptionSlice
	if oiCache != nil && oiCache.HasOI(ticker, marketDate, expiration) {
		loaded, err := oiCache.LoadOI(ticker, marketDate, expiration)
		if err == nil {
			cachedOI = loaded
		}
	}

	greeksSlice, err := client.GetOptionSlice(ctx, ticker, expiration, strikeLow, strikeHigh, false, monthlyWeight, weeklyWeight)
	if err != nil {
		return nil, err
	}

	if cachedOI != nil {
		return MergeSliceOI(greeksSlice, cachedOI), nil
	}

	oiSlice, err := client.GetOptionSlice(ctx, ticker, expiration, strikeLow, strikeHigh, true, monthlyWeight, weeklyWeight)
	if err != nil {
		return nil, err
	}
	if oiCache != nil {
		if err := oiCache.StoreOI(ticker, marketDate, expiration, oiSlice); err != nil {
			return oiSlice, fmt.Errorf("store OI cache (using fresh data): %w", err)
		}
	}
	return oiSlice, nil
}

// FetchOISlice fetches and caches OI only (no greeks) for prefetch or background OI load.
func FetchOISlice(
	ctx context.Context,
	client *polygon.Client,
	oiCache *OICache,
	settings *pkgconfluence.Settings,
	ticker, expiration string,
	marketDate time.Time,
	strikeLow, strikeHigh float64,
) (*pkgconfluence.OptionSlice, error) {
	if oiCache != nil && oiCache.HasOI(ticker, marketDate, expiration) {
		return oiCache.LoadOI(ticker, marketDate, expiration)
	}

	oiSlice, err := client.GetOptionSlice(ctx, ticker, expiration, strikeLow, strikeHigh, true,
		settings.MonthlyExpiryWeight, settings.WeeklyExpiryWeight)
	if err != nil {
		return nil, err
	}
	if oiCache != nil {
		if err := oiCache.StoreOI(ticker, marketDate, expiration, oiSlice); err != nil {
			return oiSlice, fmt.Errorf("store OI cache (using fresh data): %w", err)
		}
	}
	return oiSlice, nil
}

// MergeSliceOI overlays cached daily OI onto a greeks-only slice.
func MergeSliceOI(greeks, oi *pkgconfluence.OptionSlice) *pkgconfluence.OptionSlice {
	if greeks == nil {
		return oi
	}
	if oi == nil {
		return greeks
	}

	oiByStrike := make(map[float32]pkgconfluence.StrikeProfile, len(oi.Strikes))
	for _, sp := range oi.Strikes {
		oiByStrike[sp.Strike] = sp
	}

	merged := *greeks
	for i := range merged.Strikes {
		if cached, ok := oiByStrike[merged.Strikes[i].Strike]; ok {
			merged.Strikes[i].CallOI = cached.CallOI
			merged.Strikes[i].PutOI = cached.PutOI
		}
	}
	merged.OIAsOf = oi.OIAsOf
	return &merged
}

// FetchDayStats returns session open/high/low for a ticker on the current trading day.
func FetchDayStats(ctx context.Context, client *polygon.Client, settings *pkgconfluence.Settings, ticker string, now time.Time) (DayStats, error) {
	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	if err != nil {
		return DayStats{}, fmt.Errorf("load market timezone: %w", err)
	}
	localNow := now.In(loc)

	openClock, err := time.Parse("15:04", settings.MarketHours.Open)
	if err != nil {
		return DayStats{}, fmt.Errorf("parse market open: %w", err)
	}
	closeClock, err := time.Parse("15:04", settings.MarketHours.Close)
	if err != nil {
		return DayStats{}, fmt.Errorf("parse market close: %w", err)
	}

	sessionOpen := time.Date(localNow.Year(), localNow.Month(), localNow.Day(),
		openClock.Hour(), openClock.Minute(), 0, 0, loc)
	sessionClose := time.Date(localNow.Year(), localNow.Month(), localNow.Day(),
		closeClock.Hour(), closeClock.Minute(), 0, 0, loc)

	to := localNow
	if to.After(sessionClose) {
		to = sessionClose
	}

	if stats, ok, err := minuteDayStats(ctx, client, ticker, sessionOpen, to); err != nil {
		return DayStats{}, err
	} else if ok {
		return stats, nil
	}

	return dailyFallbackStats(ctx, client, ticker, localNow)
}

func minuteDayStats(ctx context.Context, client *polygon.Client, ticker string, from, to time.Time) (DayStats, bool, error) {
	if !to.After(from) {
		return DayStats{}, false, nil
	}

	aggs, err := client.GetAggregates(ctx, ticker, 1, "minute", from.UnixMilli(), to.UnixMilli(), true)
	if err != nil {
		return DayStats{}, false, err
	}
	if len(aggs) == 0 {
		return DayStats{}, false, nil
	}

	stats := DayStats{
		Open: aggs[0].Open,
		High: aggs[0].High,
		Low:  aggs[0].Low,
	}
	for _, bar := range aggs[1:] {
		if bar.High > stats.High {
			stats.High = bar.High
		}
		if bar.Low < stats.Low {
			stats.Low = bar.Low
		}
	}
	return stats, true, nil
}

func dailyFallbackStats(ctx context.Context, client *polygon.Client, ticker string, localNow time.Time) (DayStats, error) {
	from := localNow.AddDate(0, 0, -7)
	aggs, err := client.GetAggregates(ctx, ticker, 1, "day", from.UnixMilli(), localNow.UnixMilli(), true)
	if err != nil {
		return DayStats{}, err
	}
	if len(aggs) == 0 {
		return DayStats{}, fmt.Errorf("no daily bars for %s", ticker)
	}
	latest := aggs[len(aggs)-1]
	return DayStats{Open: latest.Open, High: latest.High, Low: latest.Low}, nil
}

// AllOIReady reports whether OI is cached for every expiration on marketDate.
func AllOIReady(oiCache *OICache, ticker string, marketDate time.Time, expirations []time.Time) bool {
	if oiCache == nil {
		return false
	}
	for _, exp := range expirations {
		if !oiCache.HasOI(ticker, marketDate, exp.Format("2006-01-02")) {
			return false
		}
	}
	return len(expirations) > 0
}
