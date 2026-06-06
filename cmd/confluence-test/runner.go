package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ekinolik/jax/internal/config"
	intconfluence "github.com/ekinolik/jax/internal/confluence"
	"github.com/ekinolik/jax/internal/polygon"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

type runnerDeps struct {
	loadConfig      func() (*config.Config, error)
	loadSettings    func(string) (*pkgconfluence.Settings, error)
	loadSICSectors  func(string) (*pkgconfluence.SICSectors, error)
	newPolygon      func(*config.Config) *polygon.Client
	newOICache      func(string) (*intconfluence.OICache, error)
	newProcessor    func(*pkgconfluence.Settings, *pkgconfluence.SICSectors, *intconfluence.OICache) *intconfluence.Processor
}

func defaultDeps() runnerDeps {
	return runnerDeps{
		loadConfig:     config.LoadConfig,
		loadSettings:   pkgconfluence.LoadSettings,
		loadSICSectors: pkgconfluence.LoadSICSectors,
		newPolygon:     polygon.NewClient,
		newOICache:     intconfluence.NewOICache,
		newProcessor: func(settings *pkgconfluence.Settings, sectors *pkgconfluence.SICSectors, oiCache *intconfluence.OICache) *intconfluence.Processor {
			registry := intconfluence.NewRegistry(settings.MaxActiveTickers)
			return intconfluence.NewProcessor(settings, sectors, registry, oiCache, nil, nil)
		},
	}
}

func run(ctx context.Context, opts cliOptions, progress io.Writer) (*pkgconfluence.ConfluenceSnapshot, error) {
	return runWithDeps(ctx, opts, progress, defaultDeps())
}

func runWithDeps(ctx context.Context, opts cliOptions, progress io.Writer, deps runnerDeps) (*pkgconfluence.ConfluenceSnapshot, error) {
	logf := func(format string, args ...interface{}) {
		if progress != nil {
			fmt.Fprintf(progress, format+"\n", args...)
		}
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	settingsPath := filepath.Join(opts.configDir, "settings.yaml")
	sectorsPath := filepath.Join(opts.configDir, "sic_sectors.yaml")

	settings, err := deps.loadSettings(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("load confluence settings: %w", err)
	}
	sectors, err := deps.loadSICSectors(sectorsPath)
	if err != nil {
		return nil, fmt.Errorf("load sic sectors: %w", err)
	}

	ticker := pkgconfluence.NormalizeTicker(opts.ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return nil, err
	}

	client := deps.newPolygon(cfg)
	oiCache, err := deps.newOICache("")
	if err != nil {
		return nil, fmt.Errorf("init OI cache: %w", err)
	}
	processor := deps.newProcessor(settings, sectors, oiCache)

	now := time.Now().UTC()
	marketDate, err := intconfluence.MarketCalendarDate(settings, now)
	if err != nil {
		return nil, err
	}

	logf("resolving expirations for %s...", ticker)
	expirations, err := client.ResolveExpirations(ctx, ticker, settings.UsesDualExpiration(ticker), marketDate)
	if err != nil {
		return nil, fmt.Errorf("resolve expirations: %w", err)
	}
	if len(expirations) == 0 {
		return nil, fmt.Errorf("no expirations found for %s", ticker)
	}

	logf("fetching spot for %s...", ticker)
	spot, spotTime, err := intconfluence.FetchLastTrade(ctx, client, ticker)
	if err != nil {
		return nil, fmt.Errorf("spot for %s: %w", ticker, err)
	}
	if spot <= 0 {
		return nil, fmt.Errorf("invalid spot price for %s", ticker)
	}

	strikeLow, strikeHigh := intconfluence.StrikeBand(spot)

	var slices []pkgconfluence.OptionSlice
	for _, exp := range expirations {
		expStr := exp.Format("2006-01-02")
		logf("loading options for %s exp %s (strikes %.2f–%.2f)...", ticker, expStr, strikeLow, strikeHigh)

		slice, err := intconfluence.FetchOptionSlice(ctx, client, oiCache, settings, ticker, expStr, marketDate, strikeLow, strikeHigh)
		if err != nil {
			return nil, fmt.Errorf("option slice %s %s: %w", ticker, expStr, err)
		}
		slices = append(slices, *slice)
	}

	logf("fetching RSI for %s...", ticker)
	rsi, _, err := client.GetRSI(ctx, ticker, 14)
	if err != nil {
		return nil, fmt.Errorf("RSI for %s: %w", ticker, err)
	}

	logf("fetching ticker overview for %s...", ticker)
	overview, err := client.GetTickerOverview(ctx, ticker)
	if err != nil {
		return nil, fmt.Errorf("ticker overview for %s: %w", ticker, err)
	}
	sectorETF := sectors.ResolveSectorETF(overview.SICCode, overview.SICDescription)
	logf("resolved sector ETF: %s", sectorETF)

	logf("fetching market benchmarks (SPY, QQQ, %s)...", sectorETF)
	spySpot, _, err := intconfluence.FetchLastTrade(ctx, client, "SPY")
	if err != nil {
		return nil, fmt.Errorf("SPY spot: %w", err)
	}
	qqqSpot, _, err := intconfluence.FetchLastTrade(ctx, client, "QQQ")
	if err != nil {
		return nil, fmt.Errorf("QQQ spot: %w", err)
	}
	etfSpot, _, err := intconfluence.FetchLastTrade(ctx, client, sectorETF)
	if err != nil {
		return nil, fmt.Errorf("%s spot: %w", sectorETF, err)
	}

	logf("fetching day range stats...")
	targetDay, err := intconfluence.FetchDayStats(ctx, client, settings, ticker, now)
	if err != nil {
		return nil, fmt.Errorf("day stats for %s: %w", ticker, err)
	}
	spyDay, err := intconfluence.FetchDayStats(ctx, client, settings, "SPY", now)
	if err != nil {
		return nil, fmt.Errorf("day stats for SPY: %w", err)
	}
	qqqDay, err := intconfluence.FetchDayStats(ctx, client, settings, "QQQ", now)
	if err != nil {
		return nil, fmt.Errorf("day stats for QQQ: %w", err)
	}
	etfDay, err := intconfluence.FetchDayStats(ctx, client, settings, sectorETF, now)
	if err != nil {
		return nil, fmt.Errorf("day stats for %s: %w", sectorETF, err)
	}

	scoreInput := pkgconfluence.ScoreInput{
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
		ETFOpen:      etfDay.Open,
		ETFSpot:      etfSpot,
		SectorETF:    sectorETF,
		IntradayHigh: targetDay.High,
		IntradayLow:  targetDay.Low,
		Now:          now,
	}

	logf("computing confluence snapshot...")
	snap, err := processor.RecomputeSnapshot(ticker, scoreInput, now)
	if err != nil {
		return nil, fmt.Errorf("recompute snapshot: %w", err)
	}
	return snap, nil
}
