package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
)

func init() {
	// Register the same types as in disk.go
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register(string(""))
	gob.Register(int(0))
	gob.Register(float64(0))
	gob.Register(bool(false))

	gob.RegisterName("LastTradeData", &cache.LastTradeData{})
	gob.RegisterName("OptionData", &cache.OptionData{})
	gob.RegisterName("AggregatesData", &cache.AggregatesData{})
	gob.RegisterName("CacheableAggregatesData", &cache.CacheableAggregatesData{})
	gob.RegisterName("CacheableAgg", &cache.CacheableAgg{})
	gob.RegisterName("CacheableAggSlice", []cache.CacheableAgg{})

	gob.Register(&polygon.LastTradeResponse{})
	gob.Register(&polygon.Chain{})
	gob.Register(models.LastTrade{})
	gob.Register(models.Agg{})
	gob.Register([]models.Agg{})
}

func main() {
	compressed := flag.Bool("compressed", true, "whether the cache file is compressed")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: cache_reader [-compressed=false] <cache_file>")
		os.Exit(1)
	}

	filePath := flag.Arg(0)

	// Read file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read cache file: %v", err)
	}

	// Decompress if needed
	var finalData []byte
	if *compressed {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			log.Fatalf("Failed to create gzip reader: %v", err)
		}
		finalData, err = ioutil.ReadAll(gz)
		if err != nil {
			log.Fatalf("Failed to decompress data: %v", err)
		}
		if err := gz.Close(); err != nil {
			log.Fatalf("Failed to close gzip reader: %v", err)
		}
	} else {
		finalData = data
	}

	// Decode data
	var wrapper cache.DataWrapper
	dec := gob.NewDecoder(bytes.NewReader(finalData))
	if err := dec.Decode(&wrapper); err != nil {
		log.Fatalf("Failed to decode data: %v", err)
	}

	// Convert to JSON for pretty printing
	jsonData, err := json.MarshalIndent(wrapper.Data, "", "  ")
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}

	fmt.Println(string(jsonData))
}
