package polygon

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/massive-com/client-go/v2/rest/models"
)

const maxOptionContracts = 2000

// GetOptionSlice fetches a filtered options chain snapshot and returns a reduced OptionSlice.
func (c *Client) GetOptionSlice(
	ctx context.Context,
	ticker, expiration string,
	strikeLow, strikeHigh float64,
	includeOI bool,
	monthlyWeight, weeklyWeight float32,
) (*confluence.OptionSlice, error) {
	if strikeHigh < strikeLow {
		return nil, fmt.Errorf("strikeHigh %.2f must be >= strikeLow %.2f", strikeHigh, strikeLow)
	}
	expDate, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration date %q: %w", expiration, err)
	}

	exp := models.Date(expDate)
	params := models.ListOptionsChainParams{
		UnderlyingAsset:  ticker,
		StrikePriceGTE:   &strikeLow,
		StrikePriceLTE:   &strikeHigh,
		ExpirationDateEQ: &exp,
	}

	iter := c.client.ListOptionsChainSnapshot(ctx, &params)
	var snapshots []models.OptionContractSnapshot
	for iter.Next() {
		snapshots = append(snapshots, iter.Item())
		if len(snapshots) > maxOptionContracts {
			return nil, fmt.Errorf("option chain snapshot exceeded %d contracts", maxOptionContracts)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("massive option chain snapshot error: %w", err)
	}

	if monthlyWeight == 0 {
		monthlyWeight = confluence.DefaultMonthlyExpiryWeight
	}
	if weeklyWeight == 0 {
		weeklyWeight = confluence.DefaultWeeklyExpiryWeight
	}

	now := time.Now().UTC()
	slice := ParseOptionSnapshots(ticker, expiration, snapshots, includeOI, now, now, monthlyWeight, weeklyWeight)
	return slice, nil
}

// ParseOptionSnapshots converts API snapshots into a reduced OptionSlice.
func ParseOptionSnapshots(
	ticker, expiration string,
	snapshots []models.OptionContractSnapshot,
	includeOI bool,
	oiAsOf, greeksAsOf time.Time,
	monthlyWeight, weeklyWeight float32,
) *confluence.OptionSlice {
	expDate, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		expDate = time.Time{}
	}
	byStrike := make(map[float32]*confluence.StrikeProfile)

	for _, snap := range snapshots {
		strike := float32(snap.Details.StrikePrice)
		profile, ok := byStrike[strike]
		if !ok {
			profile = &confluence.StrikeProfile{Strike: strike}
			byStrike[strike] = profile
		}

		switch snap.Details.ContractType {
		case "call", "CALL":
			if includeOI {
				profile.CallOI = uint32(snap.OpenInterest)
			}
			profile.CallDelta = float32(snap.Greeks.Delta)
			profile.CallGamma = float32(snap.Greeks.Gamma)
		case "put", "PUT":
			if includeOI {
				profile.PutOI = uint32(snap.OpenInterest)
			}
			profile.PutDelta = float32(snap.Greeks.Delta)
			profile.PutGamma = float32(snap.Greeks.Gamma)
		}
	}

	strikes := make([]confluence.StrikeProfile, 0, len(byStrike))
	for _, profile := range byStrike {
		strikes = append(strikes, *profile)
	}
	sort.Slice(strikes, func(i, j int) bool {
		return strikes[i].Strike < strikes[j].Strike
	})

	return &confluence.OptionSlice{
		Ticker:       ticker,
		Expiration:   expiration,
		IsMonthly:    confluence.IsMonthlyOPEX(expDate),
		ExpiryWeight: confluence.ExpiryWeightFor(expDate, monthlyWeight, weeklyWeight),
		Strikes:      strikes,
		OIAsOf:       oiAsOf,
		GreeksAsOf:   greeksAsOf,
	}
}
