package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	polygon "github.com/polygon-io/client-go/rest"
	"github.com/polygon-io/client-go/rest/iter"
	"github.com/polygon-io/client-go/rest/models"
)

type Lowest struct {
	StrikePrice string
	Ratio       float64
}

type DexEntry struct {
	Dex float64
}

type Chain map[string]map[string]map[string]models.OptionContractSnapshot
type DEX map[string]map[string]map[string]DexEntry

func main() {
	c := polygon.New(os.Getenv("POLYGON_API_KEY"))

	// set params
	params := models.ListOptionsChainParams{
		UnderlyingAsset: "AAPL",
		StrikePriceGTE:  new(float64),
		StrikePriceLTE:  new(float64),
	}.WithStrikePrice("gte", 240.0).WithStrikePrice("lte", 250.0)

	//fmt.Printf("Client: %+v\n", c.Client.HTTP.Header)
	// make request
	iter := c.ListOptionsChainSnapshot(context.Background(), params)

	chains, err := CreateChains(iter)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}

	//blah := &models.OptionContractSnapshot{}
	//low := &Lowest{StrikePrice: "", Ratio: 1000}

	//oi := CreateOpenInterestMap(chains)
	//low := FindLowestRatio(oi)

	//wtf, _ := json.MarshalIndent(chains, "", "  ")
	//fmt.Printf("%s", wtf)

	//fmt.Printf("%+v\n", oi)
	//oi_json, _ := json.MarshalIndent(oi, "", "  ")
	//fmt.Printf("%s\n", oi_json)
	//fmt.Printf("Best Call Ratio: %s - %f\n", low.StrikePrice, low.Ratio)

	dex := CreateDEXMap(chains)
	dex_json, _ := json.MarshalIndent(dex, "", "  ")
	fmt.Printf("%s\n", dex_json)
}

func CreateChains(chainIter *iter.Iter[models.OptionContractSnapshot]) (Chain, error) {
	// Creates a map of chains with the following structure:
	// strike_price -> expiration_date -> contract_type -> OptionContractSnapshot
	// OptionContractSnapshot is a struct that contains the details of the option contract with the following structure:
	// OptionContractSnapshot {
	//   BreakEvenPrice: float64
	//   ImpliedVolatility: float64
	//   OpenInterest: int
	//   Details: OptionContractDetails:
	//        ContractType: string
	//        ExerciseStyle: string
	//        ExpirationDate: time.Time ??
	//        SharesPerContract: int
	//        StrikePrice: float64
	//        Ticker: string
	//   Greeks: OptionContractGreeks:
	//        Delta: float64
	//        Gamma: float64
	//        Theta: float64
	//        Vega: float64
	//   Day: OptionContractDay:
	//        LastUpdate: time.Time
	//        High: float64
	//        Low: float64
	//        Open: float64
	//        Close: float64
	//        PrevieClose: float64
	//        Volume: int
	//        Vwap: float64
	//   LastTrade: OptionContractTrade:
	//      SipTimestamp: time.Time
	//   LastQuote: OptionContractQuote:
	//      LastUpdated: time.Time
	//   UnderlyingAsset: ????
	//      ChangeToBreakEven: float64
	//      LastUpdated: time.Time
	//      Price: float64
	//      Ticker: string
	//      Timeframe: string

	chains := make(Chain)

	for chainIter.Next() {
		//blah := &models.OptionContractSnapshot{}
		current := chainIter.Item()
		exp := current.Details.ExpirationDate
		exp_date_byte, _ := exp.MarshalJSON()
		exp_date := string(exp_date_byte[1 : len(exp_date_byte)-1])

		strike_price_float := current.Details.StrikePrice
		strike_price := strconv.FormatFloat(strike_price_float, 'f', -1, 64)

		contract_type := current.Details.ContractType

		if _, ok := chains[strike_price]; !ok {
			chains[strike_price] = map[string]map[string]models.OptionContractSnapshot{}
			chains[strike_price][exp_date] = map[string]models.OptionContractSnapshot{}
			chains[strike_price][exp_date][contract_type] = models.OptionContractSnapshot{}
		} else if _, ok := chains[strike_price][exp_date]; !ok {
			chains[strike_price][exp_date] = map[string]models.OptionContractSnapshot{}
			chains[strike_price][exp_date][contract_type] = models.OptionContractSnapshot{}
		} else if _, ok := chains[strike_price][exp_date][contract_type]; !ok {
			chains[strike_price][exp_date][contract_type] = models.OptionContractSnapshot{}
		}

		chains[strike_price][exp_date][contract_type] = current

	}

	return chains, chainIter.Err()

}

func CreateOpenInterestMap(chains Chain) map[string]map[string]float64 {
	oi := make(map[string]map[string]float64)

	for strike, v := range chains {
		if _, ok := oi[strike]; !ok {
			oi[strike] = map[string]float64{"call": 0, "put": 0, "ratio": 0}
		}

		for exp, _ := range v {
			oi[strike]["call"] += chains[strike][exp]["call"].OpenInterest
			oi[strike]["put"] += chains[strike][exp]["put"].OpenInterest
			if oi[strike]["call"] == 0 {
				oi[strike]["ratio"] = 0
			} else {
				oi[strike]["ratio"] = oi[strike]["put"] / oi[strike]["call"]
			}
		}
	}

	return oi
}

func FindLowestRatio(oi map[string]map[string]float64) Lowest {
	low := &Lowest{StrikePrice: "", Ratio: 1000}

	for strike, v := range oi {
		if v["ratio"] < low.Ratio && v["ratio"] > 0 {
			low.Ratio = v["ratio"]
			low.StrikePrice = strike
		}
	}

	return *low
}

func CreateDEXMap(chain Chain) DEX {
	dex := make(DEX)
	var dexEntry *DexEntry

	for strike, v := range chain {
		for expiration, contracts := range v {
			if _, ok := dex[strike]; !ok {
				dex[strike] = make(map[string]map[string]DexEntry)
				dex[strike][expiration] = make(map[string]DexEntry)
			} else if _, ok := dex[strike][expiration]; !ok {
				dex[strike][expiration] = make(map[string]DexEntry)
			}

			dexEntry = calculateDelta(contracts["call"])
			dex[strike][expiration]["call"] = *dexEntry

			dexEntry = calculateDelta(contracts["put"])
			dex[strike][expiration]["put"] = *dexEntry
		}
	}

	return dex
}

func calculateDelta(contract models.OptionContractSnapshot) *DexEntry {
	return &DexEntry{
		Dex: contract.OpenInterest * contract.Details.SharesPerContract * contract.Greeks.Delta * contract.UnderlyingAsset.Price,
	}
}
