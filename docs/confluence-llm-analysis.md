# Confluence Snapshot LLM Analysis Guide

How to paste JAX Confluence JSON snapshots into Claude, OpenAI, or another LLM to rank day-trading **long entry** candidates from a batch of tickers.

**Sources:** `grpcurl`, `confluence-test` CLI, jax-ov WebSocket (Phase 4), or debug HTTP (`GET /confluence/debug?ticker=...`).

**Human scan vs LLM batch:** For a quick read on one ticker, use `GetConfluenceSummary` (gRPC), `GET /confluence/summary?ticker=...` (jax-ov), or `confluence-test --summary`. The summary projects verdict, archetype, reasons, warnings, and gates from the full snapshot — no duplicate scoring. For ranking 5–20 tickers in an LLM, use the **full snapshot** batch format in Section 2 below.

---

## 1. Purpose

JAX Confluence **v2** scores each ticker on a **0–100 buy composite** from twelve weighted signals (gamma/delta support, RSI minute + daily, sector, market, upside/downside geometry, ADR, gamma regime, short squeeze) plus a parallel **sell score** for long exits. The system supports two long archetypes: **mean-reversion** (positive/neutral gamma, support entries) and **squeeze momentum** (negative gamma, above call wall, elevated SI/RVOL).

Use an LLM to:

1. Accept a **batch** of snapshots (~5–20 tickers collected in a script loop).
2. Filter stale or incomplete data.
3. **Rank** tickers for potential long entries (buy-watch) vs avoid.
4. Explain *why* using signal alignment, levels, entry timing, and range position.

The LLM does **not** replace the scorer; it interprets already-computed fields and applies trading judgment within the rubric below.

---

## 2. Input format

### Batch wrapper

Wrap snapshots in a single JSON object so the model sees one structured input:

```json
{
  "collected_at": "2026-06-06T14:32:00-04:00",
  "market_context": "RTH — US equities open",
  "snapshots": [
    { "...": "snapshot 1" },
    { "...": "snapshot 2" }
  ]
}
```

| Field | Description |
|-------|-------------|
| `collected_at` | ISO-8601 time when **you** finished collecting (not `data_as_of`). Helps staleness checks. |
| `market_context` | Free text: `"RTH open"`, `"pre-market"`, `"after close"`, etc. |
| `snapshots` | Array of one snapshot per ticker. |

### Canonical field names (gRPC / grpcurl / jax-ov)

Proto JSON uses **snake_case**. This is the preferred format for LLM prompts.

| Field | Type | Notes |
|-------|------|-------|
| `ticker` | string | Uppercase symbol |
| `timestamp` | int64 | Unix seconds — when JAX **computed** this snapshot |
| `confluence_score` | float | 0–100 composite |
| `readiness` | string | `no_trade`, `caution`, `possible_entry`, `high_conviction` |
| `oi_status` | string | `ready`, `loading`, `error` |
| `market_status` | string | `open`, `closed` |
| `signals` / `buy_signals` | array | Twelve buy axes (see glossary); `signals` mirrors `buy_signals` for compat |
| `sell_signals` | array | Six sell axes for long exits |
| `sell_score` | float | 0–100 exit conviction |
| `sell_readiness` | string | `hold`, `watch`, `consider_trim`, `take_profit` |
| `exit_action` | string | `hold`, `trim`, `sell_all` |
| `distance_to_exit` | string | `early`, `ideal`, `late` vs rank-1 resistance |
| `upside_pct` / `downside_pct` / `risk_reward` | float | Trade geometry to rank-1 resistance / second support |
| `adr_30d_pct` / `adr_5d_pct` / `adr_regime` | float/string | Dual-window ADR suitability |
| `rsi_daily` | float | RSI-14 daily (lower weight than minute) |
| `gamma_regime` / `gamma_squeeze_active` / `short_squeeze_active` | string/bool | Volatility amplifier state |
| `levels` | object | Support/resistance ladder |
| `daily_range_position` | float | 0 = at intraday low, 1 = at intraday high |
| `distance_to_entry` | string | `early`, `ideal`, `late` |
| `haptic_level` | int | 0–3 UI intensity (= readiness tier) |
| `background_level` | int | 0–3 UI intensity (= readiness tier) |
| `spot` | float | Last spot used in scoring |
| `spot_timestamp` | int64 | Unix seconds for spot quote (optional) |
| `rsi` | float | RSI-14 (minute timespan) |
| `sector_etf` | string | Benchmark ETF (from SIC map, e.g. `SMH`) |
| `stacked_zone` | bool | Two+ support levels within 1% of spot |
| `data_as_of` | int64 | Unix seconds — latest spot/greeks/OI input |
| `trade_plan` | object | Static entry playbook when `readiness` ≥ `caution` with buy setup (see below) |

### `trade_plan` vs `sell_score`

| Field | Purpose | When present |
|-------|---------|--------------|
| **`trade_plan`** | Pre-trade **entry playbook** at buy support: `entry_zone`, `stops` (soft → hard), `average_down`, `exit_instead_of_add_below`, `spot_context` | `possible_entry`, `high_conviction`, or `caution` (with note) |
| **`sell_score`** | **Profit-taking / exit** at resistance for existing longs | Always computed; independent of entry playbook |

**`trade_plan` is not live stop monitoring.** It does not track your entry price, ring buffers, or time-based exits. It answers: *if I enter at the signaled support, what levels below are stops vs average-down adds?*

- **Noise (watch):** Wick below `soft_stop` / `noise_line` — tolerance band; prefer reclaim, not panic exit.
- **Trade invalidation (day-trade exit):** Break below `cluster_floor` / `trade_failure` — local GEX cluster lost; exit the day trade (primary actionable exit for mean-reversion setups).
- **Structure invalidation (thesis dead):** Break below `structure_stop` / `thesis_failure` — DEX support lost; full thesis invalid.
- **Emergency exit:** Confirmed break below `hard_stop` / `emergency_stop` or `exit_instead_of_add_below` — do not add.

Buffers are configurable in `confluence-configs/settings.yaml` (`trade_plan.gex_stop_buffer_pct`, `trade_plan.dex_stop_buffer_pct`, `trade_plan.cluster_band_pct`, `trade_plan.gex_dex_gap_warn_pct`, `trade_plan.min_add_gap_pct`).

**Cluster-aware stops:** When multiple GEX supports cluster within `cluster_band_pct` (default 2%) below the entry anchor and the next DEX support is far below (gap > 2%), the playbook inserts a **`cluster_floor`** stop at the lowest GEX in that band. Average-down adds and `exit_instead_of_add_below` reference the cluster floor — not the distant DEX hard stop. DEX remains as `structure_stop` / `hard_stop` for final structural invalidation. When anchor-to-DEX gap exceeds `gex_dex_gap_warn_pct` (default 5%), `gex_dex_gap_pct` is set on the plan and the summary emits an air-pocket warning.

**Average-down ladder:** `average_down[0]` tier is `initial_entry` (first buy at the anchor). `add_1` uses the next real GEX support between anchor and cluster floor when one exists; otherwise, when cluster floor is active and anchor-to-floor gap ≥ `min_add_gap_pct` (default 1%), a **synthetic** `add_1` is placed at the midpoint (e.g. SNDK anchor 1600, cluster 1580 → add ~1590). `add_1` always uses `if_below: exit_instead`. Poor R/R with `caution` readiness adds an intraday note recommending a single entry while still showing the ladder for transparency.

**Summary JSON human labels:** Internal tier strings (`soft_stop`, `cluster_floor`, etc.) are unchanged for code logic. The summary `trade_plan` enriches stops with `label` / `meaning` (e.g. `trade_failure`, `thesis_failure`) and adds `invalidation.trade` / `invalidation.structure` plus `primary_exit` (emphasizes cluster floor when present — e.g. CRWD ~640, not distant emergency stop ~584).

### `confluence-test` CLI name mapping

`confluence-test` prints Go struct JSON (**PascalCase**). Map to the canonical names before prompting, or tell the LLM to treat these as aliases:

| CLI (`confluence-test`) | gRPC / grpcurl |
|-------------------------|----------------|
| `Score` | `confluence_score` |
| `ReadinessBand` | `readiness` |
| `OIStatus` | `oi_status` |
| `MarketStatus` | `market_status` |
| `RangePosition` | `daily_range_position` |
| `DistanceToEntry` | `distance_to_entry` |
| `BackgroundLevel` | `background_level` |
| `HapticLevel` | `haptic_level` |
| `SectorETF` | `sector_etf` |
| `StackedZone` | `stacked_zone` |
| `UpdatedAt` | `timestamp` (RFC3339 string in CLI vs Unix in gRPC) |
| `DataAsOf` | `data_as_of` (RFC3339 string in CLI vs Unix in gRPC) |
| `RSI` | `rsi` |

Signal and level sub-objects use the same names in both outputs (`name`, `status`, `axis_fill` / `AxisFill`, etc.).

### Collection script pattern

```bash
#!/usr/bin/env bash
# Example: grpcurl batch (local plaintext)
TICKERS=(NVDA AMD TSLA AAPL MSFT)
OUT='{"collected_at":"'"$(date -Iseconds)"'","market_context":"manual batch","snapshots":['
FIRST=1
for T in "${TICKERS[@]}"; do
  SNAP=$(grpcurl -plaintext -d "{\"ticker\":\"$T\"}" localhost:50051 jax.v1.ConfluenceService/GetConfluence)
  [[ $FIRST -eq 1 ]] && FIRST=0 || OUT+=','
  OUT+="$SNAP"
done
OUT+=']}'
echo "$OUT" | jq .
```

Paste the final JSON into the user prompt template (Section 5).

---

## 3. Field glossary

Strategy context: **day-trading mean-reversion long** — buy weakness into options-derived support when RSI is oversold and tape/sector are not hostile.

### Top-level

| Field | Meaning for long bias |
|-------|------------------------|
| `confluence_score` | Weighted sum of signal scores (0–100), plus **+5** if `stacked_zone` is true. Higher = stronger confluence for a long entry. |
| `readiness` | Tier derived from score: `no_trade` (0–34), `caution` (35–54), `possible_entry` (55–74), `high_conviction` (75–100). Primary filter before ranking. |
| `market_status` | `open` = live RTH recompute path active; `closed` = outside 9:30–16:00 ET or bootstrap/stale read (see Section 7). |
| `oi_status` | `ready` = GEX/DEX levels computed; `loading` = gamma/delta signals suppressed; `error` = treat levels as unreliable. |
| `spot` | Price used for distance-to-support and range math. |
| `rsi` | RSI-14 on minute bars. **≤ 35** is aligned (oversold) for this strategy. |
| `sector_etf` | Relative-strength benchmark (SIC → `confluence-configs/sic_sectors.yaml`). |
| `stacked_zone` | ≥2 support levels within 1% below spot — structural confluence; also boosts gamma axis and +5 score. |
| `daily_range_position` | Where spot sits in today's high–low range. **Lower is better** for mean-reversion longs (closer to intraday lows). |
| `distance_to_entry` | Spot vs nearest support: `ideal` (≤0.3% above), `early` (≤1.5%), `late` (>1.5% or below support). Prefer `ideal` or `early`. |
| `haptic_level` / `background_level` | 0–3 mirror of `readiness` (UI only); optional tie-breaker, not primary logic. |
| `timestamp` | When snapshot was built — use with `data_as_of` for freshness. |
| `data_as_of` | Latest market/options input time — critical when `market_status` is `closed`. |

### `levels` object

| Field | Meaning |
|-------|---------|
| `gamma_flip` | Strike where net gamma exposure changes sign; context for dealer positioning. |
| `support[]` / `resistance[]` | Ranked price levels from GEX (`source: "gex"`) and DEX (`source: "dex"`). Higher `rank` = more important peak. `strength` = relative peak magnitude. |
| `nearest_support` | Closest support at or below spot (if `has_nearest_support`). |
| `nearest_resistance` | Closest resistance at or above spot (if `has_nearest_resistance`). For longs, note overhead resistance distance. |

### `buy_signals[]` — twelve axes (weights sum to 100%, `confluence-configs/settings.yaml`)

Each signal has: `name`, `weight`, `axis_fill` (0–1), `status` (`aligned` / `neutral` / `against`), `icon`, `score` (axis_fill × weight × 100), `detail`.

| `name` | Weight | Aligned means (long bias) |
|--------|--------|---------------------------|
| `gamma_support` | 16% | Spot near rank-1 GEX support; stacked zone bonus |
| `delta_support` | 10% | Spot near rank-1 DEX support |
| `rsi_minute` | 12% | RSI-14 minute ≤ 35 (oversold timing) |
| `rsi_daily` | 3% | RSI-14 daily oversold (softer curve) |
| `sector` | 18% | Target not lagging sector ETF (**sector > market**) |
| `market` | 8% | SPY/QQQ not weak vs open |
| `upside` | 7% | Room to rank-1 resistance; **hard gate: <3% caps readiness at caution** |
| `downside` | 3% | Tight stop / R/R ≥ 1.5 favored |
| `adr` | 5% | Sustained 30d ADR + regime modifier; `spike_warning` caps readiness |
| `gamma_environment` | 4% | Negative gamma = trend fuel (+); positive = pinning (−) |
| `gamma_directional` | 6% | Bullish setup above call wall + VWAP + RVOL; **≤ −8 caps readiness** |
| `short_squeeze` | 8% | `pressure × trigger` (SI/DTC/short-vol × gamma/breakout fuel) |

**Compound squeeze bonus:** +10 when both `gamma_squeeze_active` and `short_squeeze_active`.

**Trade archetype tags** (LLM should assign): `mean_reversion`, `squeeze_momentum`, `mixed`, `avoid`.

### `sell_signals[]` — long exits only

| `name` | Weight | Meaning |
|--------|--------|---------|
| `resistance_proximity` | 30% | Near rank-1 resistance |
| `rsi_minute_exit` | 25% | RSI > 65–70 overbought |
| `rsi_daily_exit` | 10% | Daily RSI > 60 |
| `upside_exhausted` | 20% | <1% room to resistance |
| `market_sector_extended` | 15% | Selling into SPY/QQQ/sector strength |

**`exit_action`:** `trim` when rank-2 resistance offers ≥3% beyond rank-1 with strength ≥0.5; else `sell_all` near resistance. Squeeze unwind overlays boost sell score when gamma squeeze fades below VWAP.

### Scoring reference (v2, `confluence-configs/settings.yaml`)

- Buy readiness thresholds: 75 / 55 / 35 (unchanged)
- Stacked zone: **+5** to `confluence_score`
- `min_upside_pct: 0.03` hard gate on buy readiness
- ADR `spike_warning`: 30d < 3%, 5d ≥ 4%, ratio ≥ 1.4 → readiness capped at `caution`
- Production server: **max 5 active tickers** (`max_active_tickers: 5`)

---

## 4. System prompt

Copy everything below into the **system** role:

```
You are a quantitative day-trading assistant that ranks JAX Confluence snapshot batches for LONG entry candidates only.

Strategy: mean-reversion longs into options-derived support (GEX/DEX) when RSI is oversold, with favorable market and sector context.

Rules:
1. Input is JSON with a "snapshots" array. Each element is one ticker's ConfluenceSnapshot (snake_case gRPC fields, or PascalCase CLI aliases documented in the user message).
2. NEVER invent prices, scores, or signals not present in the JSON.
3. Apply staleness rules (user message Section 7): down-rank or exclude snapshots that are too old or collected with market_status "closed" during RTH unless the user explicitly requests after-hours planning.
4. Require oi_status "ready" for any "buy-watch" or "high_conviction" rank that cites gamma/delta support. If oi_status is "loading" or "error", cap recommendation at "monitor" and note missing levels.
5. Ranking priority (long bias):
   - readiness: high_conviction > possible_entry > caution > no_trade
   - confluence_score (higher better)
   - distance_to_entry: ideal > early > late
   - daily_range_position (lower better, closer to intraday lows)
   - Count of signals with status "aligned" (especially gamma_support, delta_support, rsi)
   - stacked_zone true is a plus
   - Penalize signals with status "against" on market or sector
6. Classify each ticker into exactly one bucket: BUY-WATCH, MONITOR, AVOID. Do not use SELL unless the user holds a position and asks for exit guidance; this system is not built for short entries.
7. Short/sell limitations: RSI "against" means overbought (bearish for new longs), not a short signal. Resistance levels may inform exits for existing longs only when the user asks.
8. Output format (markdown):
   - Brief market/data quality summary (1–2 sentences)
   - Ranked table: Rank | Ticker | Bucket | Score | Readiness | Key signals | Entry timing | One-line rationale
   - "Avoid" section for excluded tickers with reason
   - Top 1–3 actionable notes (not financial advice)
9. If all snapshots are stale or no_trade, say so clearly and recommend waiting for RTH fresh data.
10. This is educational analysis, not financial advice.
```

---

## 5. User prompt template

```
Analyze this JAX Confluence batch for day-trading LONG entry candidates (mean-reversion into GEX/DEX support).

Collection context:
- collected_at: {{COLLECTED_AT}}
- market_context: {{MARKET_CONTEXT}}

Staleness: If collected during regular US RTH (9:30–16:00 ET), prefer snapshots with market_status "open" and data_as_of within 5 minutes of collected_at. If after hours, treat as planning-only and label levels "last session".

JSON batch:
```json
{{PASTE_SNAPSHOTS_JSON_HERE}}
```

Rank all tickers. Use buckets BUY-WATCH, MONITOR, AVOID. Maximum 5 names in BUY-WATCH (matches production watch limit). Explain gamma/delta/RSI/market/sector alignment for the top picks.
```

Replace `{{COLLECTED_AT}}`, `{{MARKET_CONTEXT}}`, and paste the full batch JSON.

---

## 6. Ranking rubric

### BUY-WATCH (actively consider long entry)

All of:

- `readiness` is `possible_entry` or `high_conviction` (score ≥ 55)
- `oi_status` is `ready`
- `distance_to_entry` is `ideal` or `early`
- At least **two** of: `gamma_support`, `delta_support`, `rsi` have `status: "aligned"`
- `market` is not `against` (both SPY/QQQ not deeply red)
- Not disqualified by staleness rules (Section 7)

**Strongest:** `high_conviction` + `stacked_zone` + `ideal` + RSI aligned + `daily_range_position` < 0.4.

### MONITOR (watchlist / wait)

- `readiness` is `caution` (35–54), **or**
- Score ≥ 55 but `distance_to_entry` is `late`, **or**
- `oi_status` is `loading` (levels pending) but RSI + market look good, **or**
- One of gamma/delta against but others aligned (mixed confluence)

### AVOID (do not enter long)

Any of:

- `readiness` is `no_trade` (score < 35)
- `oi_status` is `error`
- Spot **below** nearest support (`distance_to_entry: "late"` with detail suggesting price under support)
- Three or more signals `against`
- `rsi` against (typically RSI > 55) **and** `daily_range_position` > 0.7 (chasing strength)
- `market` against (SPY and QQQ both < −0.5% vs open) unless user accepts high-risk counter-trend
- Failed staleness gate during RTH (Section 7)

### SELL / SHORT — limitations

This scorer is **long-entry biased**. It does **not** emit short signals or exit rules.

| User question | How to respond |
|---------------|----------------|
| "Should I short X?" | Decline primary short ranking; note RSI against + proximity to `nearest_resistance` only as context. |
| "Should I sell my long?" | Only if user provides position context; use `nearest_resistance`, loss of support, `readiness` dropping to `no_trade`, or `market`/`sector` turning against. |

---

## 7. Staleness rules

| Condition | Treatment |
|-----------|-----------|
| `market_status: "open"` | Live path — prefer `data_as_of` within **5 minutes** of your `collected_at` during RTH. |
| `market_status: "closed"` | Expected outside 9:30–16:00 ET or before bootstrap completes. Levels and spot may reflect **last session**. Label output **"planning only / stale"**; do not treat as live entry signal during RTH. |
| `oi_status: "loading"` | One-shot fetch in progress. Gamma/delta suppressed — **do not** rank as BUY-WATCH on structure. |
| `oi_status: "error"` | Exclude from BUY-WATCH. |
| `data_as_of` missing or 0 | Low confidence — MONITOR at best. |
| `timestamp` vs `collected_at` | Large gap means processor/cache lag; mention in summary. |

**After-hours / pre-market workflow:** Snapshots are useful to **preview** support levels and plan tickers for the open. Require fresh `market_status: "open"` data before promoting to BUY-WATCH.

**Weekend:** `data_as_of` is typically Friday's close; all ranks are planning-only.

---

## 8. Example

### Input (minimal fake batch, gRPC shape)

```json
{
  "collected_at": "2026-06-06T10:45:00-04:00",
  "market_context": "RTH open",
  "snapshots": [
    {
      "ticker": "NVDA",
      "timestamp": 1780778700,
      "confluence_score": 78.5,
      "readiness": "high_conviction",
      "oi_status": "ready",
      "market_status": "open",
      "spot": 118.42,
      "rsi": 31.2,
      "sector_etf": "SMH",
      "stacked_zone": true,
      "daily_range_position": 0.22,
      "distance_to_entry": "ideal",
      "haptic_level": 3,
      "background_level": 3,
      "data_as_of": 1780778695,
      "signals": [
        { "name": "gamma_support", "weight": 0.25, "axis_fill": 1.0, "status": "aligned", "icon": "G", "score": 25, "detail": "rank-1 GEX support 118.10 (0.27% above); stacked zone" },
        { "name": "delta_support", "weight": 0.15, "axis_fill": 0.7, "status": "neutral", "icon": "D", "score": 10.5, "detail": "rank-1 DEX support 117.50 (0.78% above)" },
        { "name": "rsi", "weight": 0.2, "axis_fill": 0.78, "status": "aligned", "icon": "R", "score": 15.6, "detail": "RSI-14 31.2" },
        { "name": "market", "weight": 0.2, "axis_fill": 0.75, "status": "aligned", "icon": "M", "score": 15, "detail": "SPY +0.15% QQQ +0.22%" },
        { "name": "sector", "weight": 0.2, "axis_fill": 0.7, "status": "neutral", "icon": "S", "score": 14, "detail": "NVDA vs SMH rel -0.10%" }
      ],
      "levels": {
        "gamma_flip": 121.0,
        "nearest_support": 118.1,
        "nearest_resistance": 120.5,
        "has_nearest_support": true,
        "has_nearest_resistance": true,
        "support": [{ "price": 118.1, "source": "gex", "strength": 0.92, "rank": 1 }],
        "resistance": [{ "price": 120.5, "source": "gex", "strength": 0.88, "rank": 1 }]
      }
    },
    {
      "ticker": "AMD",
      "timestamp": 1780778700,
      "confluence_score": 28,
      "readiness": "no_trade",
      "oi_status": "ready",
      "market_status": "open",
      "spot": 162.8,
      "rsi": 58.1,
      "sector_etf": "SMH",
      "stacked_zone": false,
      "daily_range_position": 0.81,
      "distance_to_entry": "late",
      "haptic_level": 0,
      "background_level": 0,
      "data_as_of": 1780778690,
      "signals": [
        { "name": "gamma_support", "weight": 0.25, "axis_fill": 0.2, "status": "against", "icon": "G", "score": 5, "detail": "rank-1 GEX support 158.00 (2.95% above)" },
        { "name": "delta_support", "weight": 0.15, "axis_fill": 0.2, "status": "against", "icon": "D", "score": 3, "detail": "rank-1 DEX support 157.20 (3.44% above)" },
        { "name": "rsi", "weight": 0.2, "axis_fill": 0.15, "status": "against", "icon": "R", "score": 3, "detail": "RSI-14 58.1" },
        { "name": "market", "weight": 0.2, "axis_fill": 0.75, "status": "aligned", "icon": "M", "score": 15, "detail": "SPY +0.15% QQQ +0.22%" },
        { "name": "sector", "weight": 0.2, "axis_fill": 0.4, "status": "neutral", "icon": "S", "score": 8, "detail": "AMD vs SMH rel -0.40%" }
      ],
      "levels": {
        "gamma_flip": 165.0,
        "nearest_support": 158.0,
        "nearest_resistance": 164.0,
        "has_nearest_support": true,
        "has_nearest_resistance": true,
        "support": [{ "price": 158.0, "source": "gex", "strength": 0.7, "rank": 1 }],
        "resistance": [{ "price": 164.0, "source": "gex", "strength": 0.75, "rank": 1 }]
      }
    }
  ]
}
```

### Expected LLM output (illustrative)

**Data quality:** Both snapshots are live (`market_status: open`), OI ready, `data_as_of` within 5s of collection. Suitable for RTH ranking.

| Rank | Ticker | Bucket | Score | Readiness | Key signals | Entry timing | Rationale |
|------|--------|--------|-------|-----------|-------------|--------------|-----------|
| 1 | NVDA | BUY-WATCH | 78.5 | high_conviction | G aligned, RSI aligned, M aligned; stacked zone | ideal | Oversold RSI at intraday range lows, ideal distance to GEX support, +5 stacked bonus; sector neutral only |
| 2 | AMD | AVOID | 28 | no_trade | G/D/R against | late | Extended in daily range (0.81), RSI overbought for long bias, far from support — no confluence |

**Avoid:** AMD — `no_trade`, late entry, three against signals.

**Actionable notes:** NVDA is the only candidate meeting long confluence criteria; confirm entry near 118.1 support with stop below stacked zone. Re-run after material spot move or if `market` turns against.

---

## 9. Caveats

- **Not financial advice.** Educational interpretation of JAX metrics only.
- **Day-trading context.** Signals use intraday range, day % change vs open, and minute RSI — not multi-day swing fundamentals.
- **Long bias only.** Short selling and sell signals are outside the scorer's design.
- **Production limit:** The live processor watches at most **5 tickers** (`max_active_tickers` in `confluence-configs/settings.yaml`). Batch scripts are not limited, but live streaming is.
- **API cost / rate limits:** Each ticker may trigger Massive REST calls (OI, greeks, RSI). Throttle batch collection on a t3.nano deployment.
- **Readiness bands** are fixed thresholds (35 / 55 / 75), not a `"watch"` band — use **MONITOR** for borderline cases.
- **Closed market bootstrap** can return usable levels with `market_status: "closed"` for planning; verify live data before entries.
- **Sector mapping** depends on SIC codes in `sic_sectors.yaml`; wrong ETF reduces sector signal quality.

---

## Related code

| Topic | Location |
|-------|----------|
| Snapshot types | `pkg/confluence/types.go` |
| Composite score & readiness | `pkg/confluence/score.go` |
| Signal calculators | `pkg/confluence/signals/` |
| gRPC JSON shape | `api/proto/confluence/v1/confluence.proto` |
| Processor limits | `confluence-configs/settings.yaml` |
