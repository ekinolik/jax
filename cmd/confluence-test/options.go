package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	defaultConfigDir = "confluence-configs"
	defaultFormat    = "json"
)

// cliOptions holds parsed command-line flags.
type cliOptions struct {
	ticker     string
	configDir  string
	format     string
	jsonOutput bool
}

func parseOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("confluence-test", flag.ContinueOnError)
	fs.SetOutput(osStderr())

	var ticker string
	var configDir string
	var format string
	var json bool

	fs.StringVar(&ticker, "ticker", "", "ticker symbol to score (required)")
	fs.StringVar(&configDir, "config-dir", defaultConfigDir, "directory containing settings.yaml and sic_sectors.yaml")
	fs.StringVar(&format, "format", defaultFormat, "output format: json")
	fs.BoolVar(&json, "json", true, "pretty-print JSON to stdout (alias for --format json)")

	fs.Usage = func() {
		fmt.Fprintf(osStderr(), "Usage: confluence-test --ticker TICKER [flags]\n\n")
		fmt.Fprintf(osStderr(), "Fetch live market data and print a confluence snapshot.\n\n")
		fmt.Fprintf(osStderr(), "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(osStderr(), "\nEnvironment:\n")
		fmt.Fprintf(osStderr(), "  POLYGON_API_KEY    Massive.com API key (required)\n")
		fmt.Fprintf(osStderr(), "  CONFLUENCE_CACHE_DIR  same-day OI cache directory (optional)\n")
	}

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	ticker = strings.TrimSpace(ticker)
	if ticker == "" {
		return cliOptions{}, fmt.Errorf("--ticker is required")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format != "json" {
		return cliOptions{}, fmt.Errorf("unsupported format %q (only json is supported)", format)
	}
	if !json {
		return cliOptions{}, fmt.Errorf("--json=false is not supported in v1")
	}

	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		configDir = defaultConfigDir
	}

	return cliOptions{
		ticker:     ticker,
		configDir:  configDir,
		format:     format,
		jsonOutput: true,
	}, nil
}

// osStderr is a test seam for usage output.
var osStderr = func() io.Writer { return os.Stderr }
