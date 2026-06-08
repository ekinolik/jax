package confluence

import (
	"context"
	"log"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// SnapshotNeedsLiveRefresh reports whether a warm cached snapshot should refresh
// live fields (market status, spot, minute RSI) before returning from GetConfluence.
func SnapshotNeedsLiveRefresh(settings *pkgconfluence.Settings, snap *pkgconfluence.ConfluenceSnapshot, now time.Time) bool {
	if snap == nil || settings == nil {
		return false
	}
	currentOpen, err := settings.IsRTH(now)
	if err != nil {
		return false
	}
	if currentOpen {
		return true
	}
	wasOpen := snap.MarketStatus == pkgconfluence.MarketStatusOpen
	return wasOpen && !currentOpen
}

// RefreshSnapshotLive recomputes market status, spot, and minute RSI on a warm snapshot.
// When inputs are unavailable the cached snapshot is returned unchanged except for market status.
func (p *Processor) RefreshSnapshotLive(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error) {
	return p.refreshSnapshotLiveAt(ctx, ticker, time.Now().UTC())
}

func (p *Processor) refreshSnapshotLiveAt(ctx context.Context, ticker string, now time.Time) (*pkgconfluence.ConfluenceSnapshot, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	snap, ok := p.LatestSnapshot(ticker)
	if !ok || snap == nil || SnapshotNeedsBootstrap(snap) {
		return snap, nil
	}
	if !SnapshotNeedsLiveRefresh(p.settings, snap, now) {
		return snap, nil
	}

	status, err := p.MarketStatusFor(now)
	if err != nil {
		return snap, err
	}

	in, changed, err := p.buildLiveScoreInput(ctx, snap, now, status)
	if err != nil {
		log.Printf("[confluence] live refresh input %s: %v", ticker, err)
		if snap.MarketStatus != status {
			updated := *snap
			updated.MarketStatus = status
			updated.UpdatedAt = now
			p.SetSnapshot(ticker, &updated)
			return &updated, nil
		}
		return snap, nil
	}
	inRTH, _ := p.IsRTH(now)
	if !changed && snap.MarketStatus == status && !inRTH {
		return snap, nil
	}

	refreshed, err := p.RecomputeSnapshot(ticker, in, now)
	if err != nil {
		log.Printf("[confluence] live refresh recompute %s: %v", ticker, err)
		if snap.MarketStatus != status {
			updated := *snap
			updated.MarketStatus = status
			updated.UpdatedAt = now
			p.SetSnapshot(ticker, &updated)
			return &updated, nil
		}
		return snap, err
	}
	return refreshed, nil
}

func (p *Processor) buildLiveScoreInput(
	ctx context.Context,
	snap *pkgconfluence.ConfluenceSnapshot,
	now time.Time,
	status pkgconfluence.MarketStatus,
) (pkgconfluence.ScoreInput, bool, error) {
	changed := snap.MarketStatus != status

	spot := snap.Spot
	spotTime := snap.SpotTime
	if price, ts, err := p.spotForWithTime(ctx, snap.Ticker); err == nil && price > 0 {
		if price != snap.Spot || !ts.Equal(snap.SpotTime) {
			changed = true
		}
		spot = price
		spotTime = ts
	}

	rsi := snap.RSI
	if rsiVal, _, err := p.fetchRSI(ctx, snap.Ticker); err == nil && rsiVal > 0 {
		if rsiVal != snap.RSI {
			changed = true
		}
		rsi = rsiVal
	}

	sectorETF := snap.SectorETF
	if sectorETF == "" {
		sectorETF = p.defaultSectorETF()
	}

	in := scoreInputFromSnapshot(snap, now, status, spot, spotTime, rsi, sectorETF)

	if spySpot, err := p.spotFor(ctx, "SPY"); err == nil && spySpot > 0 {
		in.SPYSpot = spySpot
	}
	if qqqSpot, err := p.spotFor(ctx, "QQQ"); err == nil && qqqSpot > 0 {
		in.QQQSpot = qqqSpot
	}
	if etfSpot, err := p.spotFor(ctx, sectorETF); err == nil && etfSpot > 0 {
		in.ETFSpot = etfSpot
	}

	if p.client != nil {
		if targetDay, err := p.dayStatsFor(ctx, snap.Ticker, now); err == nil {
			in.TargetOpen = targetDay.Open
			in.IntradayHigh = targetDay.High
			in.IntradayLow = targetDay.Low
			in.SessionOpen = targetDay.Open
			in.SessionVolume = targetDay.Volume
			in.SessionVWAP = targetDay.VWAP
		}
		if spyDay, err := p.dayStatsFor(ctx, "SPY", now); err == nil {
			in.SPYOpen = spyDay.Open
		}
		if qqqDay, err := p.dayStatsFor(ctx, "QQQ", now); err == nil {
			in.QQQOpen = qqqDay.Open
		}
		if etfDay, err := p.dayStatsFor(ctx, sectorETF, now); err == nil {
			in.ETFOpen = etfDay.Open
		}
	}

	return in, changed, nil
}

func scoreInputFromSnapshot(
	snap *pkgconfluence.ConfluenceSnapshot,
	now time.Time,
	status pkgconfluence.MarketStatus,
	spot float64,
	spotTime time.Time,
	rsi float64,
	sectorETF string,
) pkgconfluence.ScoreInput {
	in := pkgconfluence.ScoreInput{
		Ticker:           snap.Ticker,
		Spot:             spot,
		SpotTime:         spotTime,
		Slices:           append([]pkgconfluence.OptionSlice(nil), snap.Slices...),
		OIStatus:         snap.OIStatus,
		MarketStatus:     status,
		RSI:              rsi,
		RSIDaily:         snap.RSIDaily,
		SectorETF:        sectorETF,
		SessionVWAP:      snap.SessionVWAP,
		RelativeVolume:   snap.RelativeVolume,
		ShortInterestPct: snap.ShortInterestPct,
		ShortVolumeRatio: snap.ShortVolumeRatio,
		DaysToCover:      snap.DaysToCover,
		FloatShares:      snap.FloatShares,
		Now:              now,
		DataAsOf:         snap.DataAsOf,
	}
	if snap.ADR30dPct > 0 || snap.ADRRegime != "" {
		in.ADR = pkgconfluence.ADRMetrics{
			HasData:    true,
			ADR30dPct:  snap.ADR30dPct,
			ADR5dPct:   snap.ADR5dPct,
			SpikeRatio: snap.ADRSpikeRatio,
			Regime:     snap.ADRRegime,
		}
	}
	return in
}
