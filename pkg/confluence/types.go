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

// ConfluenceSnapshot is the latest computed snapshot for a watched ticker.
type ConfluenceSnapshot struct {
	Ticker       string
	Spot         float64
	SpotTime     time.Time
	Slices       []OptionSlice
	OIStatus     OIStatus
	MarketStatus MarketStatus
	UpdatedAt    time.Time
}
