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
	IconGamma  SignalIcon = "G"
	IconDelta  SignalIcon = "D"
	IconRSI    SignalIcon = "R"
	IconSector SignalIcon = "S"
	IconMarket SignalIcon = "M"
)

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

	Now time.Time
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

	Levels          Levels
	Signals         []Signal
	Score           float64
	ReadinessBand   ReadinessBand
	BackgroundLevel int
	HapticLevel     int
	StackedZone     bool
	SectorETF       string
	RangePosition   float64
	DistanceToEntry EntryTiming
	RSI             float64
}
