package polygon

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/polygon-io/client-go/rest/models"
)

// SerializedData represents data with type information
type SerializedData struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Serialize converts polygon data types to bytes with type information
func Serialize(data interface{}) ([]byte, error) {
	var jsonData []byte
	var err error
	var dataType string

	switch v := data.(type) {
	case *OptionDataEntry:
		dataType = "option_data"
		jsonData, err = json.Marshal(v)
	case *LastTradeResponse:
		dataType = "last_trade"
		jsonData, err = json.Marshal(v)
	case *AggregatesResponse:
		dataType = "aggregates"
		jsonData, err = json.Marshal(v)
	default:
		return nil, fmt.Errorf("unsupported data type: %T", data)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode data to JSON: %v", err)
	}

	wrapper := SerializedData{
		Type: dataType,
		Data: jsonData,
	}

	return json.Marshal(wrapper)
}

// Deserialize converts bytes back to the appropriate polygon data type
func Deserialize(data []byte) (interface{}, error) {
	var wrapper SerializedData
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to decode wrapper JSON: %v", err)
	}

	switch wrapper.Type {
	case "option_data":
		var data OptionDataEntry
		if err := json.Unmarshal(wrapper.Data, &data); err != nil {
			return nil, fmt.Errorf("failed to decode option data: %v", err)
		}
		return &data, nil
	case "last_trade":
		var data LastTradeResponse
		if err := json.Unmarshal(wrapper.Data, &data); err != nil {
			return nil, fmt.Errorf("failed to decode last trade data: %v", err)
		}
		return &data, nil
	case "aggregates":
		var data AggregatesResponse
		if err := json.Unmarshal(wrapper.Data, &data); err != nil {
			return nil, fmt.Errorf("failed to decode aggregates data: %v", err)
		}
		return &data, nil
	default:
		return nil, fmt.Errorf("unknown data type: %s", wrapper.Type)
	}
}

// JSONDate is a JSON-serializable version of models.Date
type JSONDate string

// JSONOptionSnapshot is a JSON-serializable version of models.OptionContractSnapshot
type JSONOptionSnapshot struct {
	BreakEvenPrice    float64                                `json:"break_even_price,omitempty"`
	Day               models.DayOptionContractSnapshot       `json:"day,omitempty"`
	Details           models.OptionDetails                   `json:"details,omitempty"`
	Greeks            models.Greeks                          `json:"greeks,omitempty"`
	ImpliedVolatility float64                                `json:"implied_volatility,omitempty"`
	LastQuote         models.LastQuoteOptionContractSnapshot `json:"last_quote,omitempty"`
	LastTrade         models.LastTradeOptionContractSnapshot `json:"last_trade,omitempty"`
	OpenInterest      float64                                `json:"open_interest,omitempty"`
	UnderlyingAsset   models.UnderlyingAsset                 `json:"underlying_asset,omitempty"`
	FairMarketValue   float64                                `json:"fmv,omitempty"`
}

// JSONChain is a JSON-serializable version of Chain
type JSONChain map[string]map[string]map[string]JSONOptionSnapshot

// toJSONChain converts a Chain to a JSONChain
func toJSONChain(c Chain) JSONChain {
	jsonChain := make(JSONChain)
	for strike, dateMap := range c {
		jsonChain[strike] = make(map[string]map[string]JSONOptionSnapshot)
		for date, contractMap := range dateMap {
			dateStr := time.Time(date).Format("2006-01-02")
			jsonChain[strike][dateStr] = make(map[string]JSONOptionSnapshot)
			for contractType, snapshot := range contractMap {
				jsonChain[strike][dateStr][contractType] = JSONOptionSnapshot{
					BreakEvenPrice:    snapshot.BreakEvenPrice,
					Day:               snapshot.Day,
					Details:           snapshot.Details,
					Greeks:            snapshot.Greeks,
					ImpliedVolatility: snapshot.ImpliedVolatility,
					LastQuote:         snapshot.LastQuote,
					LastTrade:         snapshot.LastTrade,
					OpenInterest:      snapshot.OpenInterest,
					UnderlyingAsset:   snapshot.UnderlyingAsset,
					FairMarketValue:   snapshot.FairMarketValue,
				}
			}
		}
	}
	return jsonChain
}

// toChain converts a JSONChain to a Chain
func toChain(jc JSONChain) (Chain, error) {
	c := make(Chain)
	for strike, dateMap := range jc {
		c[strike] = make(map[models.Date]map[string]models.OptionContractSnapshot)
		for dateStr, contractMap := range dateMap {
			date, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return nil, err
			}
			c[strike][models.Date(date)] = make(map[string]models.OptionContractSnapshot)
			for contractType, snapshot := range contractMap {
				c[strike][models.Date(date)][contractType] = models.OptionContractSnapshot{
					BreakEvenPrice:    snapshot.BreakEvenPrice,
					Day:               snapshot.Day,
					Details:           snapshot.Details,
					Greeks:            snapshot.Greeks,
					ImpliedVolatility: snapshot.ImpliedVolatility,
					LastQuote:         snapshot.LastQuote,
					LastTrade:         snapshot.LastTrade,
					OpenInterest:      snapshot.OpenInterest,
					UnderlyingAsset:   snapshot.UnderlyingAsset,
					FairMarketValue:   snapshot.FairMarketValue,
				}
			}
		}
	}
	return c, nil
}

// MarshalJSON implements json.Marshaler for Chain
func (c Chain) MarshalJSON() ([]byte, error) {
	return json.Marshal(toJSONChain(c))
}

// UnmarshalJSON implements json.Unmarshaler for Chain
func (c *Chain) UnmarshalJSON(data []byte) error {
	var jsonChain JSONChain
	if err := json.Unmarshal(data, &jsonChain); err != nil {
		return err
	}

	chain, err := toChain(jsonChain)
	if err != nil {
		return err
	}
	*c = chain
	return nil
}

// JSONOptionDataEntry is a JSON-serializable version of OptionDataEntry
type JSONOptionDataEntry struct {
	SpotPrice float64   `json:"spot_price"`
	Chain     JSONChain `json:"chain"`
}

// MarshalJSON implements json.Marshaler for OptionDataEntry
func (o OptionDataEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(JSONOptionDataEntry{
		SpotPrice: o.SpotPrice,
		Chain:     toJSONChain(o.Chain),
	})
}

// UnmarshalJSON implements json.Unmarshaler for OptionDataEntry
func (o *OptionDataEntry) UnmarshalJSON(data []byte) error {
	var jsonEntry JSONOptionDataEntry
	if err := json.Unmarshal(data, &jsonEntry); err != nil {
		return err
	}

	chain, err := toChain(jsonEntry.Chain)
	if err != nil {
		return err
	}

	o.SpotPrice = jsonEntry.SpotPrice
	o.Chain = chain
	return nil
}
