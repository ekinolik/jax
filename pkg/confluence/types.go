package confluence

import "time"

// StrikeProfile holds reduced per-strike options data for GEX/DEX computation.
type StrikeProfile struct {
	Strike    float32
	CallOI    uint32
	PutOI     uint32
	CallDelta float32
	PutDelta  float32
	CallGamma float32
	PutGamma  float32
}

// OptionSlice is a memory-efficient working slice for one expiration.
type OptionSlice struct {
	Ticker       string
	Expiration   string
	IsMonthly    bool
	ExpiryWeight float32
	Strikes      []StrikeProfile
	OIAsOf       time.Time
	GreeksAsOf   time.Time
}

// OIStatus tracks daily open-interest load state for a ticker.
type OIStatus string

const (
	OIStatusReady   OIStatus = "ready"
	OIStatusLoading OIStatus = "loading"
	OIStatusError   OIStatus = "error"
)

// MarketStatus indicates whether confluence processing is active.
type MarketStatus string

const (
	MarketStatusOpen   MarketStatus = "open"
	MarketStatusClosed MarketStatus = "closed"
)

// LevelSource identifies whether a support/resistance level came from GEX or DEX.
type LevelSource string

const (
	LevelSourceGEX LevelSource = "gex"
	LevelSourceDEX LevelSource = "dex"
)

// Level is a ranked support or resistance price derived from options exposure.
type Level struct {
	Price      float64
	Source     LevelSource
	Strength   float64
	Rank       int
	Expiration string
}

// Levels holds multi-peak support/resistance ladder and gamma flip.
type Levels struct {
	GammaFlip            float64
	Support              []Level
	Resistance           []Level
	NearestSupport       float64
	NearestResistance    float64
	HasNearestSupport    bool
	HasNearestResistance bool
}

// SignalStatus describes directional alignment for a confluence signal.
type SignalStatus string

const (
	SignalAligned SignalStatus = "aligned"
	SignalNeutral SignalStatus = "neutral"
	SignalAgainst SignalStatus = "against"
)

// SignalIcon is the compact radar-axis label for a signal.
type SignalIcon string

const (
	IconGamma      SignalIcon = "G"
	IconDelta      SignalIcon = "D"
	IconRSI        SignalIcon = "R"
	IconSector     SignalIcon = "S"
	IconMarket     SignalIcon = "M"
	IconUpside     SignalIcon = "U"
	IconDownside   SignalIcon = "u"
	IconADR        SignalIcon = "A"
	IconGammaEnv   SignalIcon = "g"
	IconGammaDir   SignalIcon = ">"
	IconSqueeze    SignalIcon = "Q"
	IconSell       SignalIcon = "X"
)

// ADRRegime classifies dual-window ADR suitability.
type ADRRegime string

const (
	ADRStableHigh   ADRRegime = "stable_high"
	ADRWorkable     ADRRegime = "workable"
	ADRStableLow    ADRRegime = "stable_low"
	ADRSpikeWarning ADRRegime = "spike_warning"
	ADRSpikeFade    ADRRegime = "spike_fade"
	ADRContracting  ADRRegime = "contracting"
)

// GammaRegime classifies dealer gamma positioning at spot.
type GammaRegime string

const (
	GammaPositive GammaRegime = "positive"
	GammaNegative GammaRegime = "negative"
	GammaNeutral  GammaRegime = "neutral"
)

// GammaStrength classifies normalized net GEX magnitude.
type GammaStrength string

const (
	GammaMild     GammaStrength = "mild"
	GammaModerate GammaStrength = "moderate"
	GammaExtreme  GammaStrength = "extreme"
)

// SellReadinessBand maps sell score to exit urgency tiers.
type SellReadinessBand string

const (
	SellHold         SellReadinessBand = "hold"
	SellWatch        SellReadinessBand = "watch"
	SellConsiderTrim SellReadinessBand = "consider_trim"
	SellTakeProfit   SellReadinessBand = "take_profit"
)

// ExitAction recommends trim vs full exit for longs.
type ExitAction string

const (
	ExitHold    ExitAction = "hold"
	ExitTrim    ExitAction = "trim"
	ExitSellAll ExitAction = "sell_all"
)

// ExitTiming describes spot position relative to rank-1 resistance.
type ExitTiming string

const (
	ExitEarly ExitTiming = "early"
	ExitIdeal ExitTiming = "ideal"
	ExitLate  ExitTiming = "late"
)

// DailyBar is one session OHLCV bar for ADR and breakout logic.
type DailyBar struct {
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// ADRMetrics holds dual-window ADR computation output.
type ADRMetrics struct {
	ADR30dPct   float64
	ADR5dPct    float64
	SpikeRatio  float64
	Regime      ADRRegime
	HasData     bool
}

// TradeGeometry holds upside/downside room from level ladder.
type TradeGeometry struct {
	UpsidePct     float64
	DownsidePct   float64
	RiskReward    float64
	UpsideTarget  float64
	StopSupport   float64
	HasUpside     bool
	HasDownside   bool
	StopEstimated bool
}

// Signal is one weighted axis in the confluence radar.
type Signal struct {
	Name     string
	Weight   float64
	AxisFill float64
	Status   SignalStatus
	Icon     SignalIcon
	Score    float64
	Detail   string
}

// ReadinessBand maps composite score to trade-readiness tiers.
type ReadinessBand string

const (
	ReadinessNoTrade        ReadinessBand = "no_trade"
	ReadinessCaution        ReadinessBand = "caution"
	ReadinessPossibleEntry  ReadinessBand = "possible_entry"
	ReadinessHighConviction ReadinessBand = "high_conviction"
)

// EntryTiming describes spot position relative to rank-1 support.
type EntryTiming string

const (
	EntryEarly EntryTiming = "early"
	EntryIdeal EntryTiming = "ideal"
	EntryLate  EntryTiming = "late"
)

// ScoreInput holds all inputs required to build a full ConfluenceSnapshot score.
type ScoreInput struct {
	Ticker       string
	Spot         float64
	SpotTime     time.Time
	Slices       []OptionSlice
	OIStatus     OIStatus
	MarketStatus MarketStatus
	RSI          float64
	RSIDaily     float64

	SPYOpen float64
	SPYSpot float64
	QQQOpen float64
	QQQSpot float64

	TargetOpen float64
	ETFSpot    float64
	ETFOpen    float64
	SectorETF  string

	IntradayHigh float64
	IntradayLow  float64
	SessionOpen  float64
	SessionVolume float64
	SessionVWAP  float64
	RelativeVolume float64

	ADR ADRMetrics

	ShortInterestPct float64
	ShortVolumeRatio float64
	DaysToCover      float64
	FloatShares      float64

	New20DayHigh  bool
	New52WeekHigh bool
	GapUpPct      float64

	Settings *Settings

	Now time.Time

	// DataAsOf is the latest timestamp among spot, greeks, and OI inputs (bootstrap / stale reads).
	DataAsOf time.Time
}

// ConfluenceSnapshot is the latest computed snapshot for a watched ticker.
type ConfluenceSnapshot struct {
	Ticker       string
	Spot         float64
	SpotTime     time.Time
	Slices       []OptionSlice
	OIStatus     OIStatus
	MarketStatus MarketStatus
	UpdatedAt    time.Time
	// DataAsOf is when underlying market/options inputs were last observed (may be stale outside RTH).
	DataAsOf time.Time

	Levels          Levels
	Signals         []Signal
	BuySignals      []Signal
	SellSignals     []Signal
	Score           float64
	ReadinessBand   ReadinessBand
	BackgroundLevel int
	HapticLevel     int
	StackedZone     bool
	SectorETF       string
	RangePosition   float64
	DistanceToEntry EntryTiming
	RSI             float64
	RSIDaily        float64

	// Trade geometry
	UpsidePct   float64
	DownsidePct float64
	RiskReward  float64

	// ADR dual-window
	ADRPct       float64
	ADR30dPct    float64
	ADR5dPct     float64
	ADRSpikeRatio float64
	ADRRegime    ADRRegime

	// Gamma regime + squeeze
	GammaRegime            GammaRegime
	GammaRegimeStrength    GammaStrength
	NetGEXAtSpot           float64
	CallWall               float64
	PutWall                float64
	SessionVWAP            float64
	RelativeVolume         float64
	GammaSqueezeActive     bool
	ShortSqueezeActive     bool
	GammaEnvironmentScore  float64
	GammaDirectionalScore  float64
	ShortSqueezeScore      float64
	ShortPressureScore     float64
	SqueezeTriggerScore    float64
	ShortInterestPct       float64
	ShortVolumeRatio       float64
	DaysToCover            float64
	FloatShares            float64

	// Sell path (long exits only)
	SellScore                  float64
	SellReadiness              SellReadinessBand
	ExitAction                 ExitAction
	DistanceToExit             ExitTiming
	UpsideBeyondResistancePct  float64

	// Static entry playbook (when buy readiness warrants)
	TradePlan *TradePlan
}
