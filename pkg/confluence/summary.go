package confluence

import (
	"encoding/json"
	"math"
	"time"
)

const defaultMinUpsidePct = 0.03

// TradeArchetype classifies the dominant long setup style.
type TradeArchetype string

const (
	ArchetypeMeanReversion    TradeArchetype = "mean_reversion"
	ArchetypeSqueezeMomentum  TradeArchetype = "squeeze_momentum"
	ArchetypeMixed            TradeArchetype = "mixed"
	ArchetypeAvoid            TradeArchetype = "avoid"
)

// ConfluenceSummary is a human-readable projection of a ConfluenceSnapshot.
type ConfluenceSummary struct {
	Ticker     string    `json:"ticker"`
	Spot       float64   `json:"spot"`
	AsOf       string    `json:"as_of"`
	Market     string    `json:"market"`
	Verdict    Verdict   `json:"verdict"`
	TradeSetup TradeSetup `json:"trade_setup"`
	Context    Context   `json:"context"`
	Levels     SummaryLevels `json:"levels"`
	Reasons    []string  `json:"reasons"`
	Warnings   []string  `json:"warnings"`
	Gates      Gates     `json:"gates"`
}

// Verdict holds buy and sell readiness summaries.
type Verdict struct {
	Buy  BuyVerdict  `json:"buy"`
	Sell SellVerdict `json:"sell"`
}

// BuyVerdict summarizes the buy composite.
type BuyVerdict struct {
	Score     int    `json:"score"`
	Readiness string `json:"readiness"`
	Label     string `json:"label"`
}

// SellVerdict summarizes the sell composite for long exits.
type SellVerdict struct {
	Score     int    `json:"score"`
	Readiness string `json:"readiness"`
	Action    string `json:"action"`
	Label     string `json:"label"`
}

// TradeSetup holds archetype, timing, and geometry.
type TradeSetup struct {
	Archetype    string  `json:"archetype"`
	EntryTiming  string  `json:"entry_timing"`
	ExitTiming   string  `json:"exit_timing"`
	UpsidePct    float64 `json:"upside_pct,omitempty"`
	DownsidePct  float64 `json:"downside_pct,omitempty"`
	RiskReward   float64 `json:"risk_reward,omitempty"`
}

// Context holds regime and indicator highlights.
type Context struct {
	ADRRegime      string     `json:"adr_regime"`
	GammaRegime    string     `json:"gamma_regime"`
	GammaSqueeze   bool       `json:"gamma_squeeze"`
	ShortSqueeze   bool       `json:"short_squeeze"`
	RSIMinute      RSIMinute  `json:"rsi_minute"`
	RSIDaily       float64    `json:"rsi_daily,omitempty"`
}

// RSIMinute marshals as a number or the string "unavailable".
type RSIMinute struct {
	Value         float64
	Unavailable   bool
}

// MarshalJSON implements json.Marshaler.
func (r RSIMinute) MarshalJSON() ([]byte, error) {
	if r.Unavailable {
		return json.Marshal("unavailable")
	}
	return json.Marshal(r.Value)
}

// SummaryLevels holds key price levels for quick scan.
type SummaryLevels struct {
	Support    float64 `json:"support,omitempty"`
	Resistance float64 `json:"resistance,omitempty"`
	CallWall   float64 `json:"call_wall,omitempty"`
	PutWall    float64 `json:"put_wall,omitempty"`
}

// Gates reports active buy readiness gates.
type Gates struct {
	UpsideOK                   bool `json:"upside_ok"`
	ADROK                      bool `json:"adr_ok"`
	BlockedFromHighConviction  bool `json:"blocked_from_high_conviction"`
}

// SummaryFromSnapshot projects a full snapshot into a human-readable summary.
func SummaryFromSnapshot(snap ConfluenceSnapshot) ConfluenceSummary {
	asOf := snap.DataAsOf
	if asOf.IsZero() {
		asOf = snap.UpdatedAt
	}

	return ConfluenceSummary{
		Ticker:     snap.Ticker,
		Spot:       snap.Spot,
		AsOf:       formatAsOf(asOf),
		Market:     string(snap.MarketStatus),
		Verdict:    buildVerdict(snap),
		TradeSetup: buildTradeSetup(snap),
		Context:    buildContext(snap),
		Levels:     buildSummaryLevels(snap),
		Reasons:    buildReasons(snap),
		Warnings:   buildWarnings(snap),
		Gates:      buildGates(snap),
	}
}

func formatAsOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func buildVerdict(snap ConfluenceSnapshot) Verdict {
	return Verdict{
		Buy: BuyVerdict{
			Score:     int(math.Round(snap.Score)),
			Readiness: string(snap.ReadinessBand),
			Label:     readinessLabel(snap.ReadinessBand),
		},
		Sell: SellVerdict{
			Score:     int(math.Round(snap.SellScore)),
			Readiness: string(snap.SellReadiness),
			Action:    string(snap.ExitAction),
			Label:     sellReadinessLabel(snap.SellReadiness),
		},
	}
}

func buildTradeSetup(snap ConfluenceSnapshot) TradeSetup {
	setup := TradeSetup{
		Archetype:   string(deriveArchetype(snap)),
		EntryTiming: string(snap.DistanceToEntry),
		ExitTiming:  string(snap.DistanceToExit),
	}
	if snap.UpsidePct > 0 {
		setup.UpsidePct = roundPct(snap.UpsidePct)
	}
	if snap.DownsidePct > 0 {
		setup.DownsidePct = roundPct(snap.DownsidePct)
	}
	if snap.RiskReward > 0 {
		setup.RiskReward = round1(snap.RiskReward)
	}
	return setup
}

func buildContext(snap ConfluenceSnapshot) Context {
	ctx := Context{
		ADRRegime:    string(snap.ADRRegime),
		GammaRegime:  string(snap.GammaRegime),
		GammaSqueeze: snap.GammaSqueezeActive,
		ShortSqueeze: snap.ShortSqueezeActive,
		RSIMinute:    rsiMinuteField(snap.RSI),
	}
	if snap.RSIDaily > 0 {
		ctx.RSIDaily = round1(snap.RSIDaily)
	}
	return ctx
}

func buildSummaryLevels(snap ConfluenceSnapshot) SummaryLevels {
	lvl := SummaryLevels{
		CallWall: snap.CallWall,
		PutWall:  snap.PutWall,
	}
	if snap.Levels.HasNearestSupport {
		lvl.Support = snap.Levels.NearestSupport
	}
	if snap.Levels.HasNearestResistance {
		lvl.Resistance = snap.Levels.NearestResistance
	}
	return lvl
}

func rsiMinuteField(rsi float64) RSIMinute {
	if rsi <= 0 {
		return RSIMinute{Unavailable: true}
	}
	return RSIMinute{Value: round1(rsi)}
}

func readinessLabel(band ReadinessBand) string {
	switch band {
	case ReadinessHighConviction:
		return "Strong buy setup"
	case ReadinessPossibleEntry:
		return "Watch for entry"
	case ReadinessCaution:
		return "Wait — conditions weak"
	default:
		return "Avoid"
	}
}

func sellReadinessLabel(band SellReadinessBand) string {
	switch band {
	case SellTakeProfit:
		return "Take profit"
	case SellConsiderTrim:
		return "Consider trimming"
	case SellWatch:
		return "Watch for exit"
	default:
		return "No exit signal"
	}
}

func deriveArchetype(snap ConfluenceSnapshot) TradeArchetype {
	if snap.ReadinessBand == ReadinessNoTrade {
		return ArchetypeAvoid
	}

	meanRev := isMeanReversionSetup(snap)
	squeeze := isSqueezeMomentumSetup(snap)

	switch {
	case meanRev && squeeze:
		return ArchetypeMixed
	case squeeze:
		return ArchetypeSqueezeMomentum
	case meanRev:
		return ArchetypeMeanReversion
	case snap.ReadinessBand == ReadinessCaution && countAlignedBuySignals(snap) == 0:
		return ArchetypeAvoid
	default:
		return ArchetypeMixed
	}
}

func isMeanReversionSetup(snap ConfluenceSnapshot) bool {
	gammaOK := snap.GammaRegime == GammaPositive || snap.GammaRegime == GammaNeutral
	entryOK := snap.DistanceToEntry == EntryIdeal || snap.DistanceToEntry == EntryEarly || snap.StackedZone
	supportOK := snap.Levels.HasNearestSupport
	return gammaOK && entryOK && supportOK
}

func isSqueezeMomentumSetup(snap ConfluenceSnapshot) bool {
	if snap.GammaRegime != GammaNegative {
		return false
	}
	aboveCallWall := snap.CallWall > 0 && snap.Spot > snap.CallWall
	elevatedFuel := snap.GammaSqueezeActive || snap.ShortSqueezeActive ||
		snap.RelativeVolume >= 1.5 || snap.ShortInterestPct >= 10
	return aboveCallWall && elevatedFuel
}

func buildReasons(snap ConfluenceSnapshot) []string {
	var reasons []string

	if snap.StackedZone {
		reasons = append(reasons, "Stacked support zone within 1% of spot")
	}
	if snap.DistanceToEntry == EntryIdeal {
		reasons = appendUnique(reasons, "Spot at ideal support entry distance")
	}
	if snap.GammaSqueezeActive {
		reasons = appendUnique(reasons, "Gamma squeeze active — momentum fuel")
	}
	if snap.ShortSqueezeActive {
		reasons = appendUnique(reasons, "Short squeeze pressure building")
	}

	for _, sig := range buySignals(snap) {
		if len(reasons) >= 4 {
			break
		}
		if sig.Status != SignalAligned {
			continue
		}
		if msg := alignedSignalReason(sig.Name); msg != "" {
			reasons = appendUnique(reasons, msg)
		}
	}

	return capStrings(reasons, 4)
}

func alignedSignalReason(name string) string {
	switch name {
	case "gamma_support":
		return "Gamma support aligned near spot"
	case "delta_support":
		return "Delta support at nearest level"
	case "rsi_minute":
		return "Minute RSI oversold"
	case "rsi_daily":
		return "Daily RSI supportive"
	case "sector":
		return "Sector relative strength positive"
	case "market":
		return "Broad market supportive"
	case "upside":
		return "Adequate upside room to resistance"
	case "downside":
		return "Tight downside with favorable risk/reward"
	case "adr":
		return "ADR regime supports day-trade range"
	case "gamma_environment":
		return "Negative gamma environment fuels momentum"
	case "gamma_directional":
		return "Bullish gamma directional setup"
	case "short_squeeze":
		return "Short squeeze score aligned"
	default:
		return ""
	}
}

func buildWarnings(snap ConfluenceSnapshot) []string {
	var warnings []string

	if snap.Levels.HasNearestResistance && snap.UpsidePct > 0 && snap.UpsidePct < defaultMinUpsidePct {
		warnings = append(warnings, "Upside room below 3% minimum — readiness capped")
	}
	if snap.ADRRegime == ADRSpikeWarning {
		warnings = append(warnings, "ADR spike warning — readiness capped at caution")
	}
	if snap.GammaDirectionalScore <= -8 {
		warnings = append(warnings, "Gamma directional score weak — readiness capped")
	}
	if snap.RSI <= 0 {
		warnings = append(warnings, "Minute RSI unavailable")
	}
	switch snap.OIStatus {
	case OIStatusLoading:
		warnings = append(warnings, "Open interest still loading — gamma/delta levels suppressed")
	case OIStatusError:
		warnings = append(warnings, "Open interest error — levels unreliable")
	}

	for _, sig := range buySignals(snap) {
		if sig.Status != SignalAgainst {
			continue
		}
		if msg := againstSignalWarning(sig.Name); msg != "" {
			warnings = appendUnique(warnings, msg)
		}
	}

	return capStrings(warnings, 6)
}

func againstSignalWarning(name string) string {
	switch name {
	case "market":
		return "Broad market working against long entry"
	case "sector":
		return "Sector relative strength negative"
	case "gamma_support", "delta_support":
		return "Options-derived support not aligned"
	case "rsi_minute":
		return "Minute RSI not oversold"
	case "upside":
		return "Limited upside room to resistance"
	default:
		return ""
	}
}

func buildGates(snap ConfluenceSnapshot) Gates {
	rawBand := BandForScore(snap.Score)
	return Gates{
		UpsideOK:                  upsideOK(snap),
		ADROK:                     snap.ADRRegime != ADRSpikeWarning,
		BlockedFromHighConviction: rawBand == ReadinessHighConviction && snap.ReadinessBand != ReadinessHighConviction,
	}
}

func upsideOK(snap ConfluenceSnapshot) bool {
	if !snap.Levels.HasNearestResistance || snap.UpsidePct <= 0 {
		return true
	}
	return snap.UpsidePct >= defaultMinUpsidePct
}

func buySignals(snap ConfluenceSnapshot) []Signal {
	if len(snap.BuySignals) > 0 {
		return snap.BuySignals
	}
	return snap.Signals
}

func countAlignedBuySignals(snap ConfluenceSnapshot) int {
	n := 0
	for _, sig := range buySignals(snap) {
		if sig.Status == SignalAligned {
			n++
		}
	}
	return n
}

func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

func capStrings(in []string, max int) []string {
	if len(in) <= max {
		if in == nil {
			return []string{}
		}
		return in
	}
	return in[:max]
}

func roundPct(v float64) float64 {
	return math.Round(v*1000) / 10 // one decimal as percent display (0.042 -> 4.2 if stored as ratio)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
