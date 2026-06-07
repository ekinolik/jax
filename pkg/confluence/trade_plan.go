package confluence

import (
	"fmt"
	"math"
	"sort"
)

// TradePlan is a static entry playbook anchored to buy support at snapshot time.
type TradePlan struct {
	EntryZone                  EntryZone   `json:"entry_zone"`
	Stops                      []StopLevel `json:"stops"`
	AverageDown                []AddLevel  `json:"average_down"`
	ExitInsteadOfAddBelow      float64     `json:"exit_instead_of_add_below,omitempty"`
	SpotContext                SpotContext `json:"spot_context"`
	IntradayNotes              []string    `json:"intraday_notes,omitempty"`
	GEXDEXGapPct               float64     `json:"gex_dex_gap_pct,omitempty"`
	TradeInvalidationPrice     float64     `json:"trade_invalidation_price,omitempty"`
	StructureInvalidationPrice float64     `json:"structure_invalidation_price,omitempty"`
	PrimaryExitPrice           float64     `json:"primary_exit_price,omitempty"`
}

// EntryZone identifies where buy/average-in is justified.
type EntryZone struct {
	Price               float64 `json:"price"`
	Source              string  `json:"source"`
	Timing              string  `json:"timing"`
	DistanceFromSpotPct float64 `json:"distance_from_spot_pct,omitempty"`
	Note                string  `json:"note,omitempty"`
}

// StopLevel is one ordered failure price below the entry anchor.
type StopLevel struct {
	Tier       string  `json:"tier"`
	Price      float64 `json:"price"`
	Source     string  `json:"source"`
	Rule       string  `json:"rule"`
	Action     string  `json:"action"`
	HumanLabel string  `json:"label,omitempty"`
	Meaning    string  `json:"meaning,omitempty"`
}

// AddLevel is a planned average-down price between anchor and hard stop.
type AddLevel struct {
	Tier      string  `json:"tier"`
	Price     float64 `json:"price"`
	SizeHint  string  `json:"size_hint"`
	Condition string  `json:"condition"`
	IfBelow   string  `json:"if_below,omitempty"`
}

// SpotContext compares live spot to the published plan (not position-aware).
type SpotContext struct {
	VsEntryZone string `json:"vs_entry_zone"`
	VsSoftStop  string `json:"vs_soft_stop"`
	VsHardStop  string `json:"vs_hard_stop"`
	Guidance    string `json:"guidance"`
}

// ClusterFloor returns the lowest GEX support within bandPct below anchor.
func ClusterFloor(levels Levels, anchor float64, bandPct float64) (float64, bool) {
	if anchor <= 0 || bandPct <= 0 {
		return 0, false
	}
	floor := anchor * (1 - bandPct)
	var lowest float64
	found := false
	for _, lvl := range levels.Support {
		if lvl.Source != LevelSourceGEX || lvl.Price >= anchor || lvl.Price < floor {
			continue
		}
		if !found || lvl.Price < lowest {
			lowest = lvl.Price
			found = true
		}
	}
	return lowest, found
}

// BuildTradePlan derives a static entry playbook when buy readiness warrants it.
func BuildTradePlan(snap ConfluenceSnapshot, cfg TradePlanConfig) *TradePlan {
	if snap.ReadinessBand == ReadinessNoTrade {
		return nil
	}
	if snap.ReadinessBand != ReadinessPossibleEntry &&
		snap.ReadinessBand != ReadinessHighConviction &&
		snap.ReadinessBand != ReadinessCaution {
		return nil
	}

	cfg.applyDefaults()

	anchor, source := selectEntryAnchor(snap)
	if anchor <= 0 {
		return nil
	}

	stops, hardStop, clusterFloor, useClusterFloor, gexDexGap := buildTradeStops(snap, anchor, cfg)
	adds, exitBelow := buildAverageDown(anchor, hardStop, clusterFloor, useClusterFloor, source, snap.StackedZone, snap.Levels, cfg)

	softPrice := stopPriceByTier(stops, "soft_stop")
	spotCtx := buildSpotContext(snap.Spot, anchor, softPrice, hardStop, snap.ReadinessBand)

	entry := EntryZone{
		Price:  anchor,
		Source: source,
		Timing: string(snap.DistanceToEntry),
	}
	if snap.Spot > 0 {
		entry.DistanceFromSpotPct = roundPct((snap.Spot - anchor) / snap.Spot)
	}
	if snap.ReadinessBand == ReadinessCaution {
		entry.Note = "setup not fully confirmed"
	}

	plan := &TradePlan{
		EntryZone:             entry,
		Stops:                 stops,
		AverageDown:           adds,
		ExitInsteadOfAddBelow: exitBelow,
		SpotContext:           spotCtx,
		GEXDEXGapPct:          gexDexGap,
	}
	enrichTradePlanDisplay(plan)
	return plan
}

// StopTierDisplay maps an internal stop tier to a human label and short meaning.
func StopTierDisplay(tier string) (label, meaning string) {
	switch tier {
	case "soft_stop":
		return "noise_line", "Wick tolerance — watch"
	case "cluster_floor":
		return "trade_failure", "Local GEX cluster lost — exit day trade"
	case "structure_stop":
		return "thesis_failure", "DEX structure lost — thesis invalid"
	case "hard_stop":
		return "emergency_stop", "Confirmed break below DEX"
	case "final_stop":
		return "emergency_stop", "Deepest backup"
	case "stacked_stop":
		return "stacked_stop", "Stacked zone support lost"
	default:
		return tier, ""
	}
}

// StopTierTitleLabel returns a title-case label for invalidation summaries.
func StopTierTitleLabel(tier string) string {
	switch tier {
	case "cluster_floor":
		return "Trade failure"
	case "structure_stop":
		return "Thesis failure"
	case "soft_stop":
		return "Noise line"
	case "hard_stop", "final_stop":
		return "Emergency stop"
	case "stacked_stop":
		return "Stacked stop"
	default:
		return tier
	}
}

func enrichTradePlanDisplay(plan *TradePlan) {
	cluster := stopPriceByTier(plan.Stops, "cluster_floor")
	structStop := stopPriceByTier(plan.Stops, "structure_stop")
	plan.TradeInvalidationPrice = cluster
	plan.StructureInvalidationPrice = structStop
	switch {
	case cluster > 0:
		plan.PrimaryExitPrice = cluster
	case structStop > 0:
		plan.PrimaryExitPrice = structStop
	}
	for i := range plan.Stops {
		plan.Stops[i].HumanLabel, plan.Stops[i].Meaning = StopTierDisplay(plan.Stops[i].Tier)
	}
}

func selectEntryAnchor(snap ConfluenceSnapshot) (float64, string) {
	if lvl, ok := rank1GEXSupport(snap.Levels); ok {
		return lvl.Price, "gex_support"
	}
	if snap.PutWall > 0 {
		return snap.PutWall, "put_wall"
	}
	if snap.Levels.HasNearestSupport && snap.Levels.NearestSupport > 0 {
		return snap.Levels.NearestSupport, "nearest_support"
	}
	return 0, ""
}

func rank1GEXSupport(levels Levels) (Level, bool) {
	for _, lvl := range levels.Support {
		if lvl.Source == LevelSourceGEX {
			return lvl, true
		}
	}
	return Level{}, false
}

func dexSupportBelowAnchor(levels Levels, anchor float64) (Level, bool) {
	for _, lvl := range levels.Support {
		if lvl.Source == LevelSourceDEX && lvl.Price < anchor {
			return lvl, true
		}
	}
	return Level{}, false
}

func lowestStackedSupport(levels Levels, spot float64) (float64, bool) {
	if spot <= 0 || len(levels.Support) < 2 {
		return 0, false
	}
	band := spot * 0.01
	var lowest float64
	found := false
	for _, lvl := range levels.Support {
		if spot-lvl.Price <= band && spot >= lvl.Price {
			if !found || lvl.Price < lowest {
				lowest = lvl.Price
				found = true
			}
		}
	}
	return lowest, found
}

func nearestSupportBelow(levels Levels, ceiling float64) (Level, bool) {
	var best Level
	found := false
	for _, lvl := range levels.Support {
		if lvl.Price >= ceiling {
			continue
		}
		if !found || lvl.Price > best.Price {
			best = lvl
			found = true
		}
	}
	return best, found
}

func supportsBetween(levels Levels, high, low float64) []Level {
	var out []Level
	for _, lvl := range levels.Support {
		if lvl.Price < high && lvl.Price > low {
			out = append(out, lvl)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Price > out[j].Price
	})
	return out
}

func buildTradeStops(snap ConfluenceSnapshot, anchor float64, cfg TradePlanConfig) ([]StopLevel, float64, float64, bool, float64) {
	var stops []StopLevel
	var hardStop float64
	var gexDexGap float64

	softPrice := roundPrice(anchor * (1 - cfg.GEXStopBufferPct))
	stops = append(stops, StopLevel{
		Tier:   "soft_stop",
		Price:  softPrice,
		Source: "gex",
		Rule:   fmt.Sprintf("%.1f%% below entry GEX %.2f", cfg.GEXStopBufferPct*100, anchor),
		Action: "watch",
	})

	clusterFloor, hasCluster := ClusterFloor(snap.Levels, anchor, cfg.ClusterBandPct)
	dexLvl, hasDEX := dexSupportBelowAnchor(snap.Levels, anchor)

	useClusterFloor := false
	if hasCluster && hasDEX {
		gapToDEX := (clusterFloor - dexLvl.Price) / clusterFloor
		if gapToDEX > cfg.ClusterDEXMinGapPct {
			useClusterFloor = true
			stops = append(stops, StopLevel{
				Tier:   "cluster_floor",
				Price:  clusterFloor,
				Source: "gex",
				Rule:   fmt.Sprintf("lowest GEX in %.0f%% band below anchor", cfg.ClusterBandPct*100),
				Action: "exit_if_lost",
			})
		}
	}

	if hasDEX {
		if gap := (anchor - dexLvl.Price) / anchor; gap >= cfg.GEXDEXGapWarnPct {
			gexDexGap = gap
		}
		stops = append(stops, StopLevel{
			Tier:   "structure_stop",
			Price:  dexLvl.Price,
			Source: "dex",
			Rule:   "rank-1 DEX support",
			Action: "exit_if_lost",
		})
		hardStop = roundPrice(dexLvl.Price * (1 - cfg.DEXStopBufferPct))
		stops = append(stops, StopLevel{
			Tier:   "hard_stop",
			Price:  hardStop,
			Source: "dex",
			Rule:   fmt.Sprintf("%.1f%% below DEX %.2f", cfg.DEXStopBufferPct*100, dexLvl.Price),
			Action: "exit",
		})
	}

	if snap.StackedZone {
		if low, ok := lowestStackedSupport(snap.Levels, snap.Spot); ok && low < anchor {
			stops = append(stops, StopLevel{
				Tier:   "stacked_stop",
				Price:  low,
				Source: "ladder",
				Rule:   "lowest support in stacked cluster",
				Action: "exit_if_lost",
			})
		}
	}

	finalCeiling := hardStop
	if finalCeiling <= 0 {
		finalCeiling = anchor
	}
	if finalLvl, ok := nearestSupportBelow(snap.Levels, finalCeiling); ok {
		if !stopTierExists(stops, "structure_stop", finalLvl.Price) &&
			!stopTierExists(stops, "hard_stop", finalLvl.Price) {
			stops = append(stops, StopLevel{
				Tier:   "final_stop",
				Price:  finalLvl.Price,
				Source: string(finalLvl.Source),
				Rule:   "next support below hard stop",
				Action: "exit",
			})
		}
	}

	stops = dedupeStops(stops)
	sort.Slice(stops, func(i, j int) bool {
		return stops[i].Price > stops[j].Price
	})
	return stops, hardStop, clusterFloor, useClusterFloor, gexDexGap
}

func stopTierExists(stops []StopLevel, tier string, price float64) bool {
	for _, s := range stops {
		if s.Tier == tier && math.Abs(s.Price-price) < 0.01 {
			return true
		}
	}
	return false
}

func dedupeStops(stops []StopLevel) []StopLevel {
	seen := make(map[string]bool)
	out := make([]StopLevel, 0, len(stops))
	for _, s := range stops {
		key := fmt.Sprintf("%s:%.2f", s.Tier, s.Price)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func buildAverageDown(anchor, hardStop, clusterFloor float64, useClusterFloor bool, anchorSource string, stacked bool, levels Levels, cfg TradePlanConfig) ([]AddLevel, float64) {
	adds := []AddLevel{{
		Tier:      "starter",
		Price:     anchor,
		SizeHint:  "small",
		Condition: starterCondition(anchorSource),
	}}

	exitBelow := hardStop
	addFloor := hardStop
	if useClusterFloor && clusterFloor > 0 {
		exitBelow = roundPrice(clusterFloor * (1 - cfg.GEXStopBufferPct))
		addFloor = clusterFloor
	}

	mid := supportsBetween(levels, anchor, addFloor)
	if len(mid) >= 1 && addFloor > 0 {
		s := mid[0]
		cond := fmt.Sprintf("Add only if still above exit level %.2f and reclaims %.0f", exitBelow, s.Price)
		if useClusterFloor {
			cond = fmt.Sprintf("Add only if cluster floor %.0f holds and reclaims %.0f", clusterFloor, s.Price)
		}
		adds = append(adds, AddLevel{
			Tier:      "add_1",
			Price:     s.Price,
			SizeHint:  "small",
			Condition: cond,
			IfBelow:   "exit_instead",
		})
	}
	if stacked && len(mid) >= 2 && addFloor > 0 {
		s := mid[1]
		adds = append(adds, AddLevel{
			Tier:      "add_2",
			Price:     s.Price,
			SizeHint:  "small",
			Condition: "Last small add — only if stacked zone intact",
			IfBelow:   "exit_instead",
		})
	}
	return adds, exitBelow
}

func starterCondition(source string) string {
	switch source {
	case "gex_support":
		return "Buy/average-in at rank-1 GEX support (ideal entry)"
	case "put_wall":
		return "Buy/average-in at put wall support"
	default:
		return "Buy/average-in at nearest support when readiness ≥ possible_entry"
	}
}

func buildSpotContext(spot, anchor, softStop, hardStop float64, band ReadinessBand) SpotContext {
	ctx := SpotContext{
		VsEntryZone: compareSpotToLevel(spot, anchor),
		VsSoftStop:  compareSpotToLevel(spot, softStop),
		VsHardStop:  compareSpotToLevel(spot, hardStop),
	}

	switch {
	case hardStop > 0 && spot < hardStop:
		ctx.Guidance = "Below hard stop — playbook was for entry higher; do not add — consider exit"
	case anchor > 0 && spot > 0 && (spot-anchor)/spot > 0.01:
		ctx.Guidance = fmt.Sprintf("Above entry zone — wait for pullback toward %.0f", anchor)
	case band == ReadinessCaution:
		ctx.Guidance = "Near entry zone — playbook applies with caution; setup not fully confirmed"
	case ctx.VsEntryZone == "at" || ctx.VsEntryZone == "above":
		ctx.Guidance = "At entry zone — playbook applies if you enter here"
	default:
		ctx.Guidance = "Below entry zone — wait for reclaim before using add levels"
	}
	return ctx
}

func compareSpotToLevel(spot, level float64) string {
	if level <= 0 || spot <= 0 {
		return ""
	}
	tolerance := level * 0.001
	if math.Abs(spot-level) <= tolerance {
		return "at"
	}
	if spot > level {
		return "above"
	}
	return "below"
}

func stopPriceByTier(stops []StopLevel, tier string) float64 {
	for _, s := range stops {
		if s.Tier == tier {
			return s.Price
		}
	}
	return 0
}

func roundPrice(v float64) float64 {
	return math.Round(v*100) / 100
}

// TradePlanReasonLine returns a one-line summary of the entry playbook for reasons.
func TradePlanReasonLine(plan *TradePlan) string {
	if plan == nil {
		return ""
	}
	entry := plan.EntryZone.Price
	soft := stopPriceByTier(plan.Stops, "soft_stop")
	cluster := stopPriceByTier(plan.Stops, "cluster_floor")
	structStop := stopPriceByTier(plan.Stops, "structure_stop")
	hard := plan.ExitInsteadOfAddBelow

	var addPrice float64
	for _, a := range plan.AverageDown {
		if a.Tier == "add_1" {
			addPrice = a.Price
			break
		}
	}

	var line string
	switch {
	case cluster > 0 && structStop > 0:
		line = fmt.Sprintf("Enter ~%.0f; exit day trade if cluster fails ~%.0f; thesis dead below %.0f (DEX)",
			entry, cluster, structStop)
	case structStop > 0:
		line = fmt.Sprintf("Enter ~%.0f; thesis dead below %.0f (DEX)", entry, structStop)
	case cluster > 0:
		line = fmt.Sprintf("Enter ~%.0f; exit day trade if cluster fails ~%.0f", entry, cluster)
	default:
		line = fmt.Sprintf("Enter ~%.0f", entry)
	}

	if hard > 0 && cluster == 0 {
		line += fmt.Sprintf("; emergency exit %.1f", hard)
	} else if soft > 0 && cluster == 0 && structStop == 0 {
		line += fmt.Sprintf("; watch noise below %.1f", soft)
	}

	if addPrice > 0 {
		ref := cluster
		if ref <= 0 {
			ref = plan.ExitInsteadOfAddBelow
		}
		if ref <= 0 {
			ref = hard
		}
		if ref > 0 {
			if cluster > 0 {
				line += fmt.Sprintf("; add at %.0f only if %.0f holds and reclaims %.0f", addPrice, ref, addPrice)
			} else {
				line += fmt.Sprintf("; add at %.0f only if %.1f holds and reclaims %.0f", addPrice, ref, addPrice)
			}
		}
	}
	return line
}
