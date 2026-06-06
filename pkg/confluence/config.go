package confluence

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scmhub/calendar"
	"gopkg.in/yaml.v3"
)

const defaultSettingsPath = "confluence-configs/settings.yaml"
const defaultSICSectorsPath = "confluence-configs/sic_sectors.yaml"

// MarketHours defines regular trading hours for confluence processing.
type MarketHours struct {
	Timezone string `yaml:"timezone"`
	Open     string `yaml:"open"`
	Close    string `yaml:"close"`
}

// APIRetryConfig controls exponential backoff for Massive REST calls.
type APIRetryConfig struct {
	MaxRetries  int `yaml:"max_retries"`
	BaseDelayMs int `yaml:"base_delay_ms"`
}

// TuningConfig holds rate-limit knobs tuned for t3.nano deployment.
type TuningConfig struct {
	GreeksIntervalSec      int `yaml:"greeks_interval_sec"`
	RecomputeDebounceSec   int `yaml:"recompute_debounce_sec"`
	MaxRSICallsPerMinute   int `yaml:"max_rsi_calls_per_minute"`
}

// SignalWeights holds per-axis buy signal weights (should sum to 1.0).
type SignalWeights struct {
	GammaSupport      float64 `yaml:"gamma_support"`
	DeltaSupport      float64 `yaml:"delta_support"`
	RSIMinute         float64 `yaml:"rsi_minute"`
	RSIDaily          float64 `yaml:"rsi_daily"`
	Sector            float64 `yaml:"sector"`
	Market            float64 `yaml:"market"`
	Upside            float64 `yaml:"upside"`
	Downside          float64 `yaml:"downside"`
	ADR               float64 `yaml:"adr"`
	GammaEnvironment  float64 `yaml:"gamma_environment"`
	GammaDirectional  float64 `yaml:"gamma_directional"`
	ShortSqueeze      float64 `yaml:"short_squeeze"`
}

// ScoringConfig holds v2 scoring thresholds and gates.
type ScoringConfig struct {
	MinUpsidePct            float64 `yaml:"min_upside_pct"`
	UpsideGreatPct          float64 `yaml:"upside_great_pct"`
	ADRSpikeRatioWarn       float64 `yaml:"adr_spike_ratio_warn"`
	ADR5dSpikeFloorPct      float64 `yaml:"adr_5d_spike_floor_pct"`
	ADR30dSpikeCeilingPct   float64 `yaml:"adr_30d_spike_ceiling_pct"`
	MinRiskReward           float64 `yaml:"min_risk_reward"`
	NeutralGammaBandPct     float64 `yaml:"neutral_gamma_band_pct"`
	CompoundSqueezeBonus    float64 `yaml:"compound_squeeze_bonus"`
	GammaDirectionalBuyCap  float64 `yaml:"gamma_directional_buy_cap"`
}

// Settings holds confluence engine configuration from settings.yaml.
type Settings struct {
	PrefetchWatchlist      []string         `yaml:"prefetch_watchlist"`
	DualExpirationTickers  []string         `yaml:"dual_expiration_tickers"`
	MaxActiveTickers       int              `yaml:"max_active_tickers"`
	OIPrefetchTime         string           `yaml:"oi_prefetch_time"`
	MarketHours            MarketHours      `yaml:"market_hours"`
	MonthlyExpiryWeight    float32          `yaml:"monthly_expiry_weight"`
	WeeklyExpiryWeight     float32          `yaml:"weekly_expiry_weight"`
	APIRetry               APIRetryConfig   `yaml:"api_retry"`
	Tuning                 TuningConfig     `yaml:"tuning"`
	SignalWeights          SignalWeights    `yaml:"signal_weights"`
	Scoring                ScoringConfig    `yaml:"scoring"`
}

// SICSectorMapping maps SIC codes or descriptions to sector ETFs.
type SICSectorMapping struct {
	SICCodes                  []string `yaml:"sic_codes"`
	SICDescriptionContains    string   `yaml:"sic_description_contains"`
	SectorETF                 string   `yaml:"sector_etf"`
}

// SICSectors holds SIC-to-ETF mappings from sic_sectors.yaml.
type SICSectors struct {
	Mappings         []SICSectorMapping `yaml:"mappings"`
	DefaultSectorETF string             `yaml:"default_sector_etf"`
}

// LoadSettings reads confluence settings from the given path.
func LoadSettings(path string) (*Settings, error) {
	if path == "" {
		path = defaultSettingsPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read confluence settings: %w", err)
	}

	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse confluence settings: %w", err)
	}
	settings.applyDefaults()
	return &settings, nil
}

// LoadSICSectors reads SIC sector mappings from the given path.
func LoadSICSectors(path string) (*SICSectors, error) {
	if path == "" {
		path = defaultSICSectorsPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sic sectors config: %w", err)
	}

	var sectors SICSectors
	if err := yaml.Unmarshal(data, &sectors); err != nil {
		return nil, fmt.Errorf("parse sic sectors config: %w", err)
	}
	if sectors.DefaultSectorETF == "" {
		sectors.DefaultSectorETF = "IWM"
	}
	return &sectors, nil
}

func (s *Settings) applyDefaults() {
	if s.MaxActiveTickers == 0 {
		s.MaxActiveTickers = 5
	}
	if s.MarketHours.Timezone == "" {
		s.MarketHours.Timezone = "America/New_York"
	}
	if s.MarketHours.Open == "" {
		s.MarketHours.Open = "09:30"
	}
	if s.MarketHours.Close == "" {
		s.MarketHours.Close = "16:00"
	}
	if s.MonthlyExpiryWeight == 0 {
		s.MonthlyExpiryWeight = DefaultMonthlyExpiryWeight
	}
	if s.WeeklyExpiryWeight == 0 {
		s.WeeklyExpiryWeight = DefaultWeeklyExpiryWeight
	}
	if s.APIRetry.MaxRetries == 0 {
		s.APIRetry.MaxRetries = 5
	}
	if s.APIRetry.BaseDelayMs == 0 {
		s.APIRetry.BaseDelayMs = 500
	}
	if s.Tuning.GreeksIntervalSec == 0 {
		s.Tuning.GreeksIntervalSec = 90
	}
	if s.Tuning.RecomputeDebounceSec == 0 {
		s.Tuning.RecomputeDebounceSec = 5
	}
	if s.Tuning.MaxRSICallsPerMinute == 0 {
		s.Tuning.MaxRSICallsPerMinute = 12
	}
	s.SignalWeights.applyDefaults()
	s.Scoring.applyDefaults()
}

func (w *SignalWeights) applyDefaults() {
	defaults := DefaultSignalWeights()
	if w.GammaSupport == 0 {
		w.GammaSupport = defaults.GammaSupport
	}
	if w.DeltaSupport == 0 {
		w.DeltaSupport = defaults.DeltaSupport
	}
	if w.RSIMinute == 0 {
		w.RSIMinute = defaults.RSIMinute
	}
	if w.RSIDaily == 0 {
		w.RSIDaily = defaults.RSIDaily
	}
	if w.Sector == 0 {
		w.Sector = defaults.Sector
	}
	if w.Market == 0 {
		w.Market = defaults.Market
	}
	if w.Upside == 0 {
		w.Upside = defaults.Upside
	}
	if w.Downside == 0 {
		w.Downside = defaults.Downside
	}
	if w.ADR == 0 {
		w.ADR = defaults.ADR
	}
	if w.GammaEnvironment == 0 {
		w.GammaEnvironment = defaults.GammaEnvironment
	}
	if w.GammaDirectional == 0 {
		w.GammaDirectional = defaults.GammaDirectional
	}
	if w.ShortSqueeze == 0 {
		w.ShortSqueeze = defaults.ShortSqueeze
	}
}

func (c *ScoringConfig) applyDefaults() {
	if c.MinUpsidePct == 0 {
		c.MinUpsidePct = 0.03
	}
	if c.UpsideGreatPct == 0 {
		c.UpsideGreatPct = 0.06
	}
	if c.ADRSpikeRatioWarn == 0 {
		c.ADRSpikeRatioWarn = 1.4
	}
	if c.ADR5dSpikeFloorPct == 0 {
		c.ADR5dSpikeFloorPct = 4.0
	}
	if c.ADR30dSpikeCeilingPct == 0 {
		c.ADR30dSpikeCeilingPct = 3.0
	}
	if c.MinRiskReward == 0 {
		c.MinRiskReward = 1.5
	}
	if c.NeutralGammaBandPct == 0 {
		c.NeutralGammaBandPct = 0.003
	}
	if c.CompoundSqueezeBonus == 0 {
		c.CompoundSqueezeBonus = 10
	}
	if c.GammaDirectionalBuyCap == 0 {
		c.GammaDirectionalBuyCap = -8
	}
}

// DefaultSignalWeights returns v2 buy signal weight defaults.
func DefaultSignalWeights() SignalWeights {
	return SignalWeights{
		GammaSupport:     0.16,
		DeltaSupport:     0.10,
		RSIMinute:        0.12,
		RSIDaily:         0.03,
		Sector:           0.18,
		Market:           0.08,
		Upside:           0.07,
		Downside:         0.03,
		ADR:              0.05,
		GammaEnvironment: 0.04,
		GammaDirectional: 0.06,
		ShortSqueeze:     0.08,
	}
}

// DefaultScoringConfig returns v2 scoring threshold defaults.
func DefaultScoringConfig() ScoringConfig {
	cfg := ScoringConfig{}
	cfg.applyDefaults()
	return cfg
}

// EffectiveSettings returns settings or defaults when nil (tests / CLI).
func EffectiveSettings(s *Settings) Settings {
	if s == nil {
		out := Settings{}
		out.SignalWeights = DefaultSignalWeights()
		out.Scoring = DefaultScoringConfig()
		return out
	}
	return *s
}

var (
	nyseCalendarMu sync.Mutex
	nyseCalendar   *calendar.Calendar
	nyseCalendarY  int
)

func nyseCalForYear(year int) *calendar.Calendar {
	nyseCalendarMu.Lock()
	defer nyseCalendarMu.Unlock()
	if nyseCalendar != nil && (nyseCalendarY == year || nyseCalendarY == year-1) {
		return nyseCalendar
	}
	nyseCalendar = calendar.XNYS(year, year+1)
	nyseCalendarY = year
	return nyseCalendar
}

// IsTradingDay reports whether the NYSE is open on the market-timezone calendar date of now.
func (s *Settings) IsTradingDay(now time.Time) (bool, error) {
	loc, err := time.LoadLocation(s.MarketHours.Timezone)
	if err != nil {
		return false, fmt.Errorf("load market timezone: %w", err)
	}
	local := now.In(loc)
	cal := nyseCalForYear(local.Year())
	return cal.IsBusinessDay(local), nil
}

// UsesDualExpiration reports whether ticker should fetch two expirations for OI.
func (s *Settings) UsesDualExpiration(ticker string) bool {
	ticker = NormalizeTicker(ticker)
	for _, t := range s.DualExpirationTickers {
		if NormalizeTicker(t) == ticker {
			return true
		}
	}
	return false
}

// IsRTH reports whether now falls within configured regular trading hours on an NYSE trading day.
func (s *Settings) IsRTH(now time.Time) (bool, error) {
	tradingDay, err := s.IsTradingDay(now)
	if err != nil {
		return false, err
	}
	if !tradingDay {
		return false, nil
	}

	loc, err := time.LoadLocation(s.MarketHours.Timezone)
	if err != nil {
		return false, fmt.Errorf("load market timezone: %w", err)
	}

	local := now.In(loc)
	open, err := parseClock(local, s.MarketHours.Open)
	if err != nil {
		return false, err
	}
	close, err := parseClock(local, s.MarketHours.Close)
	if err != nil {
		return false, err
	}
	return !local.Before(open) && local.Before(close), nil
}

// ResolveSectorETF maps SIC metadata to a sector benchmark ETF.
func (sectors *SICSectors) ResolveSectorETF(sicCode, sicDescription string) string {
	for _, mapping := range sectors.Mappings {
		for _, code := range mapping.SICCodes {
			if code == sicCode {
				return mapping.SectorETF
			}
		}
		if mapping.SICDescriptionContains != "" && containsFold(sicDescription, mapping.SICDescriptionContains) {
			return mapping.SectorETF
		}
	}
	return sectors.DefaultSectorETF
}

func parseClock(day time.Time, clock string) (time.Time, error) {
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse market hours time %q: %w", clock, err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, day.Location()), nil
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToUpper(haystack), strings.ToUpper(needle))
}
