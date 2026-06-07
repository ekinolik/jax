package confluence

import (
	"fmt"
	"math"
	"sort"
)

// TradePlan is a static entry playbook anchored to buy support at snapshot time.
type TradePlan struct {
	EntryZone             EntryZone   `json:"entry_zone"`
	Stops                 []StopLevel `json:"stops"`
	AverageDown           []AddLevel  `json:"average_down"`
	ExitInsteadOfAddBelow float64     `json:"exit_instead_of_add_below,omitempty"`
	SpotContext           SpotContext `json:"spot_context"`
	IntradayNotes         []string    `json:"intraday_notes,omitempty"`
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
	Tier   string  `json:"tier"`
	Price  float64 `json:"price"`
	Source string  `json:"source"`
	Rule   string  `json:"rule"`
	Action string  `json:"action"`
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

var defaultIntradayNotes = []string{
	"Brief pierce of soft stop may be noise — prefer reclaim over single tick",
	"Lunch (12–1 ET) and last 15 min often see mean-reversion bounces",
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

	stops, hardStop := buildTradeStops(snap, anchor, cfg)
	adds, exitBelow := buildAverageDown(anchor, hardStop, source, snap.StackedZone, snap.Levels)

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

	return &TradePlan{
		EntryZone:             entry,
		Stops:                 stops,
		AverageDown:           adds,
		ExitInsteadOfAddBelow: exitBelow,
		SpotContext:           spotCtx,
		IntradayNotes:         append([]string(nil), defaultIntradayNotes...),
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

func buildTradeStops(snap ConfluenceSnapshot, anchor float64, cfg TradePlanConfig) ([]StopLevel, float64) {
	var stops []StopLevel
	var hardStop float64

	softPrice := roundPrice(anchor * (1 - cfg.GEXStopBufferPct))
	stops = append(stops, StopLevel{
		Tier:   "soft_stop",
		Price:  softPrice,
		Source: "gex",
		Rule:   fmt.Sprintf("%.1f%% below entry GEX %.2f", cfg.GEXStopBufferPct*100, anchor),
		Action: "watch",
	})

	if dexLvl, ok := dexSupportBelowAnchor(snap.Levels, anchor); ok {
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
	return stops, hardStop
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

func buildAverageDown(anchor, hardStop float64, anchorSource string, stacked bool, levels Levels) ([]AddLevel, float64) {
	adds := []AddLevel{{
		Tier:      "starter",
		Price:     anchor,
		SizeHint:  "small",
		Condition: starterCondition(anchorSource),
	}}

	exitBelow := hardStop
	mid := supportsBetween(levels, anchor, hardStop)
	if len(mid) >= 1 && hardStop > 0 {
		s := mid[0]
		adds = append(adds, AddLevel{
			Tier:      "add_1",
			Price:     s.Price,
			SizeHint:  "small",
			Condition: fmt.Sprintf("Add only if still above hard stop %.2f and reclaims %.0f", hardStop, anchor),
			IfBelow:   "exit_instead",
		})
	}
	if stacked && len(mid) >= 2 && hardStop > 0 {
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
	hard := plan.ExitInsteadOfAddBelow

	var addPrice float64
	for _, a := range plan.AverageDown {
		if a.Tier == "add_1" {
			addPrice = a.Price
			break
		}
	}

	if hard > 0 && addPrice > 0 {
		return fmt.Sprintf("Enter ~%.0f; soft stop %.1f; hard exit %.1f; add at %.0f only on reclaim",
			entry, soft, hard, addPrice)
	}
	if hard > 0 {
		return fmt.Sprintf("Enter ~%.0f; soft stop %.1f; hard exit %.1f", entry, soft, hard)
	}
	if soft > 0 {
		return fmt.Sprintf("Enter ~%.0f; soft stop %.1f", entry, soft)
	}
	return fmt.Sprintf("Enter ~%.0f", entry)
}
