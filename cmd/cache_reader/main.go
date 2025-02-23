package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

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

	// Parse JSON data
	var result interface{}
	if err := json.Unmarshal(finalData, &result); err != nil {
		log.Fatalf("Failed to parse JSON data: %v", err)
	}

	// Pretty print the JSON
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to format JSON: %v", err)
	}

	fmt.Println(string(prettyJSON))
}
