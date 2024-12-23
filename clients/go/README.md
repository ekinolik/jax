# JAX Go Client

A Go client library for the JAX (Options Delta Exposure) service.

## Installation

```bash
go get github.com/ekinolik/jax/clients/go
```

## Usage

```go
package main

import (
    "context"
    "log"
    
    jax "github.com/ekinolik/jax/clients/go"
)

func main() {
    // Create a new client
    client, err := jax.NewClient("localhost:50051")
    if err != nil {
        log.Fatalf("Failed to create client: %v", err)
    }
    defer client.Close()

    // Example parameters
    underlyingAsset := "AAPL"
    startStrike := 170.0
    endStrike := 180.0

    // Get DEX data
    response, err := client.GetDex(context.Background(), underlyingAsset, &startStrike, &endStrike)
    if err != nil {
        log.Fatalf("Failed to get DEX data: %v", err)
    }

    // Use the response
    log.Printf("Spot Price: %f", response.SpotPrice)
    for strike, expDates := range response.StrikePrices {
        // Process strike prices and expiration dates
    }
}
```

## Response Structure

The response includes:
- `SpotPrice`: Current price of the underlying asset
- `StrikePrices`: Map of strike prices to expiration dates and option types, with DEX values

Example response:
```json
{
  "spotPrice": 123.45,
  "strikePrices": {
    "170": {
      "expirationDates": {
        "2024-03-15": {
          "optionTypes": {
            "call": {
              "value": 12345.67
            },
            "put": {
              "value": -1234.56
            }
          }
        }
      }
    }
  }
}
``` 