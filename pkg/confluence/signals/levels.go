package signals

import (
	"math"
	"sort"

	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	peakMinFraction   = 0.20
	mergeZoneFraction = 0.005
	gexSourceWeight   = 1.0
	dexSourceWeight   = 0.85
)

type strikeExposure struct {
	strike       float64
	netGEX       float64
	netDEX       float64
	expiration   string
	expiryWeight float64
}

type rawPeak struct {
	strike       float64
	magnitude    float64
	source       confluence.LevelSource
	expiration   string
	expiryWeight float64
}

// ComputeLevels derives multi-peak support/resistance from OptionSlices and live spot.
func ComputeLevels(slices []confluence.OptionSlice, spot float64) confluence.Levels {
	if spot <= 0 || len(slices) == 0 {
		return confluence.Levels{}
	}

	exposures := buildExposures(slices, spot)
	if len(exposures) == 0 {
		return confluence.Levels{}
	}

	gammaFlip := findGammaFlip(exposures, spot)
	gexPeaks := collectPeaks(exposures, spot, confluence.LevelSourceGEX)
	dexPeaks := collectPeaks(exposures, spot, confluence.LevelSourceDEX)

	allPeaks := append(gexPeaks, dexPeaks...)
	support := buildLadder(allPeaks, spot, true)
	resistance := buildLadder(allPeaks, spot, false)

	levels := confluence.Levels{
		GammaFlip:  gammaFlip,
		Support:    support,
		Resistance: resistance,
	}
	if nearest, ok := nearestByPrice(support, spot, true); ok {
		levels.NearestSupport = nearest
		levels.HasNearestSupport = true
	}
	if nearest, ok := nearestByPrice(resistance, spot, false); ok {
		levels.NearestResistance = nearest
		levels.HasNearestResistance = true
	}
	return levels
}

func buildExposures(slices []confluence.OptionSlice, spot float64) []strikeExposure {
	byStrike := make(map[float64]*strikeExposure)

	for _, slice := range slices {
		weight := float64(slice.ExpiryWeight)
		if weight <= 0 {
			weight = 1
		}
		for _, sp := range slice.Strikes {
			strike := float64(sp.Strike)
			entry, ok := byStrike[strike]
			if !ok {
				entry = &strikeExposure{strike: strike, expiration: slice.Expiration, expiryWeight: weight}
				byStrike[strike] = entry
			}
			entry.netGEX += StrikeGEX(sp, spot) * weight
			entry.netDEX += StrikeDEX(sp, spot) * weight
			if weight > entry.expiryWeight {
				entry.expiration = slice.Expiration
				entry.expiryWeight = weight
			}
		}
	}

	result := make([]strikeExposure, 0, len(byStrike))
	for _, e := range byStrike {
		result = append(result, *e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].strike < result[j].strike
	})
	return result
}

func findGammaFlip(exposures []strikeExposure, spot float64) float64 {
	if len(exposures) < 2 || spot <= 0 {
		return 0
	}

	bestDist := math.MaxFloat64
	var flip float64
	for i := 0; i < len(exposures)-1; i++ {
		a, b := exposures[i], exposures[i+1]
		if a.netGEX == 0 {
			if dist := math.Abs(a.strike - spot); dist < bestDist {
				bestDist = dist
				flip = a.strike
			}
			continue
		}
		if b.netGEX == 0 {
			if dist := math.Abs(b.strike - spot); dist < bestDist {
				bestDist = dist
				flip = b.strike
			}
			continue
		}
		if (a.netGEX > 0 && b.netGEX < 0) || (a.netGEX < 0 && b.netGEX > 0) {
			// Linear interpolation for zero crossing.
			ratio := a.netGEX / (a.netGEX - b.netGEX)
			cross := a.strike + ratio*(b.strike-a.strike)
			if dist := math.Abs(cross - spot); dist < bestDist {
				bestDist = dist
				flip = cross
			}
		}
	}
	return flip
}

func nearestByPrice(levels []confluence.Level, spot float64, below bool) (float64, bool) {
	if len(levels) == 0 || spot <= 0 {
		return 0, false
	}
	best := levels[0].Price
	bestDist := math.Abs(spot - best)
	for _, lvl := range levels[1:] {
		dist := math.Abs(spot - lvl.Price)
		if dist < bestDist {
			bestDist = dist
			best = lvl.Price
		}
	}
	if below && best >= spot {
		return 0, false
	}
	if !below && best <= spot {
		return 0, false
	}
	return best, true
}

func collectPeaks(exposures []strikeExposure, spot float64, source confluence.LevelSource) []rawPeak {
	values := make([]float64, len(exposures))
	for i, e := range exposures {
		switch source {
		case confluence.LevelSourceGEX:
			values[i] = e.netGEX
		default:
			values[i] = e.netDEX
		}
	}

	support := findDirectionalPeaks(exposures, values, spot, source, true)
	resistance := findDirectionalPeaks(exposures, values, spot, source, false)
	return filterAndMergePeaks(append(support, resistance...), spot)
}

func findDirectionalPeaks(exposures []strikeExposure, values []float64, spot float64, source confluence.LevelSource, below bool) []rawPeak {
	var peaks []rawPeak
	for i := range values {
		strike := exposures[i].strike
		if below && strike >= spot {
			continue
		}
		if !below && strike <= spot {
			continue
		}
		if values[i] <= 0 {
			continue
		}
		if !isLocalMaximum(values, i) {
			continue
		}
		peaks = append(peaks, rawPeak{
			strike:       strike,
			magnitude:    values[i],
			source:       source,
			expiration:   exposures[i].expiration,
			expiryWeight: exposures[i].expiryWeight,
		})
	}
	return peaks
}

func isLocalMaximum(values []float64, i int) bool {
	if i > 0 && values[i] <= values[i-1] {
		return false
	}
	if i < len(values)-1 && values[i] <= values[i+1] {
		return false
	}
	return true
}

func filterAndMergePeaks(peaks []rawPeak, spot float64) []rawPeak {
	if len(peaks) == 0 {
		return nil
	}

	bySource := make(map[confluence.LevelSource][]rawPeak)
	for _, p := range peaks {
		bySource[p.source] = append(bySource[p.source], p)
	}

	var merged []rawPeak
	for _, group := range bySource {
		supportPeaks := filterDirection(group, spot, true)
		resistancePeaks := filterDirection(group, spot, false)
		merged = append(merged, mergeNearbyPeaks(supportPeaks, spot)...)
		merged = append(merged, mergeNearbyPeaks(resistancePeaks, spot)...)
	}
	return merged
}

func filterDirection(peaks []rawPeak, spot float64, below bool) []rawPeak {
	var directed []rawPeak
	for _, p := range peaks {
		if below && p.strike >= spot {
			continue
		}
		if !below && p.strike <= spot {
			continue
		}
		directed = append(directed, p)
	}
	if len(directed) == 0 {
		return nil
	}

	maxMag := 0.0
	for _, p := range directed {
		if p.magnitude > maxMag {
			maxMag = p.magnitude
		}
	}
	if maxMag <= 0 {
		return nil
	}

	threshold := maxMag * peakMinFraction
	filtered := make([]rawPeak, 0, len(directed))
	for _, p := range directed {
		if p.magnitude >= threshold {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func mergeNearbyPeaks(peaks []rawPeak, spot float64) []rawPeak {
	if len(peaks) == 0 {
		return nil
	}
	sort.Slice(peaks, func(i, j int) bool {
		return peaks[i].strike < peaks[j].strike
	})

	mergeDist := spot * mergeZoneFraction
	merged := []rawPeak{peaks[0]}
	for i := 1; i < len(peaks); i++ {
		last := &merged[len(merged)-1]
		if peaks[i].source != last.source {
			merged = append(merged, peaks[i])
			continue
		}
		if math.Abs(peaks[i].strike-last.strike) <= mergeDist {
			if peaks[i].magnitude > last.magnitude {
				*last = peaks[i]
			}
			continue
		}
		merged = append(merged, peaks[i])
	}
	return merged
}

func buildLadder(peaks []rawPeak, spot float64, support bool) []confluence.Level {
	bySource := make(map[confluence.LevelSource][]rawPeak)
	for _, p := range peaks {
		bySource[p.source] = append(bySource[p.source], p)
	}

	type scored struct {
		peak  rawPeak
		score float64
	}
	var scoredPeaks []scored

	for source, group := range bySource {
		directed := filterDirection(group, spot, support)
		if len(directed) == 0 {
			continue
		}

		maxMag := 0.0
		for _, p := range directed {
			if p.magnitude > maxMag {
				maxMag = p.magnitude
			}
		}

		sourceW := gexSourceWeight
		if source == confluence.LevelSourceDEX {
			sourceW = dexSourceWeight
		}

		for _, p := range directed {
			score := (p.magnitude / maxMag) * proximityFactor(p.strike, spot) * p.expiryWeight * sourceW
			scoredPeaks = append(scoredPeaks, scored{peak: p, score: score})
		}
	}

	if len(scoredPeaks) == 0 {
		return nil
	}

	if support {
		sort.Slice(scoredPeaks, func(i, j int) bool {
			if scoredPeaks[i].score != scoredPeaks[j].score {
				return scoredPeaks[i].score > scoredPeaks[j].score
			}
			return scoredPeaks[i].peak.strike > scoredPeaks[j].peak.strike
		})
	} else {
		sort.Slice(scoredPeaks, func(i, j int) bool {
			if scoredPeaks[i].score != scoredPeaks[j].score {
				return scoredPeaks[i].score > scoredPeaks[j].score
			}
			return scoredPeaks[i].peak.strike < scoredPeaks[j].peak.strike
		})
	}

	levels := make([]confluence.Level, 0, len(scoredPeaks))
	for i, sp := range scoredPeaks {
		strength := sp.score
		if strength > 1 {
			strength = 1
		}
		levels = append(levels, confluence.Level{
			Price:      sp.peak.strike,
			Source:     sp.peak.source,
			Strength:   strength,
			Rank:       i + 1,
			Expiration: sp.peak.expiration,
		})
	}
	return levels
}

func proximityFactor(strike, spot float64) float64 {
	if spot <= 0 {
		return 0
	}
	dist := math.Abs(strike-spot) / spot
	return 1.0 / (1.0 + dist*8)
}

// HasStackedZone reports whether two or more support levels sit within 1% of spot.
func HasStackedZone(levels confluence.Levels, spot float64) bool {
	if spot <= 0 || len(levels.Support) < 2 {
		return false
	}
	band := spot * 0.01
	count := 0
	for _, lvl := range levels.Support {
		if spot-lvl.Price <= band && spot >= lvl.Price {
			count++
		}
	}
	return count >= 2
}

// Rank1GEXSupport returns the highest-ranked GEX support below spot, if any.
func Rank1GEXSupport(levels confluence.Levels) (confluence.Level, bool) {
	for _, lvl := range levels.Support {
		if lvl.Source == confluence.LevelSourceGEX {
			return lvl, true
		}
	}
	return confluence.Level{}, false
}

// Rank1DEXSupport returns the highest-ranked DEX support below spot, if any.
func Rank1DEXSupport(levels confluence.Levels) (confluence.Level, bool) {
	for _, lvl := range levels.Support {
		if lvl.Source == confluence.LevelSourceDEX {
			return lvl, true
		}
	}
	return confluence.Level{}, false
}

// Rank1Resistance returns the strongest resistance level (rank 1) above spot.
func Rank1Resistance(levels confluence.Levels) (confluence.Level, bool) {
	if len(levels.Resistance) == 0 {
		return confluence.Level{}, false
	}
	return levels.Resistance[0], true
}

// Rank2Resistance returns the second-ranked resistance level, if present.
func Rank2Resistance(levels confluence.Levels) (confluence.Level, bool) {
	if len(levels.Resistance) < 2 {
		return confluence.Level{}, false
	}
	return levels.Resistance[1], true
}

// SecondSupport returns the second-ranked support below spot (stop reference).
func SecondSupport(levels confluence.Levels) (confluence.Level, bool) {
	if len(levels.Support) < 2 {
		return confluence.Level{}, false
	}
	return levels.Support[1], true
}

// Rank1GEXResistance returns the highest-ranked GEX resistance above spot.
func Rank1GEXResistance(levels confluence.Levels) (confluence.Level, bool) {
	for _, lvl := range levels.Resistance {
		if lvl.Source == confluence.LevelSourceGEX {
			return lvl, true
		}
	}
	return confluence.Level{}, false
}

// Rank1DEXResistance returns the highest-ranked DEX resistance above spot.
func Rank1DEXResistance(levels confluence.Levels) (confluence.Level, bool) {
	for _, lvl := range levels.Resistance {
		if lvl.Source == confluence.LevelSourceDEX {
			return lvl, true
		}
	}
	return confluence.Level{}, false
}

// TradeGeometry computes upside/downside room from the level ladder.
func TradeGeometry(spot float64, levels confluence.Levels) confluence.TradeGeometry {
	geo := confluence.TradeGeometry{}
	if spot <= 0 {
		return geo
	}

	var upsideTarget float64
	if r1, ok := Rank1Resistance(levels); ok && r1.Price > spot {
		upsideTarget = r1.Price
		geo.HasUpside = true
	} else if levels.HasNearestResistance && levels.NearestResistance > spot {
		upsideTarget = levels.NearestResistance
		geo.HasUpside = true
	}
	if geo.HasUpside {
		geo.UpsideTarget = upsideTarget
		geo.UpsidePct = (upsideTarget - spot) / spot
	}

	var stopPrice float64
	if s2, ok := SecondSupport(levels); ok && s2.Price < spot {
		stopPrice = s2.Price
		geo.HasDownside = true
	} else if levels.GammaFlip > 0 && levels.GammaFlip < spot {
		dist := (spot - levels.GammaFlip) / spot
		if dist <= 0.05 {
			stopPrice = levels.GammaFlip
			geo.HasDownside = true
		}
	}
	if !geo.HasDownside {
		stopPrice = spot * 0.98
		geo.HasDownside = true
		geo.StopEstimated = true
	}
	geo.StopSupport = stopPrice
	geo.DownsidePct = (spot - stopPrice) / spot

	denom := math.Max(geo.DownsidePct, 0.001)
	if geo.HasUpside {
		geo.RiskReward = geo.UpsidePct / denom
	}
	return geo
}
