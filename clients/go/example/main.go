package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

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

	// Pretty print the response
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal response: %v", err)
	}

	fmt.Fprintf(os.Stdout, "Response:\n%s\n", string(jsonData))
}
