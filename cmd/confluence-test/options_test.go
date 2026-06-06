package main

import (
	"bytes"
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOptions_requiresTicker(t *testing.T) {
	_, err := parseOptions(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ticker is required")
}

func TestParseOptions_normalizesTicker(t *testing.T) {
	opts, err := parseOptions([]string{"--ticker", " nvda "})
	require.NoError(t, err)
	assert.Equal(t, "nvda", opts.ticker)
	assert.Equal(t, defaultConfigDir, opts.configDir)
	assert.True(t, opts.jsonOutput)
	assert.Equal(t, "json", opts.format)
}

func TestParseOptions_customConfigDir(t *testing.T) {
	opts, err := parseOptions([]string{"--ticker", "SPY", "--config-dir", "custom-configs"})
	require.NoError(t, err)
	assert.Equal(t, "custom-configs", opts.configDir)
}

func TestParseOptions_rejectsUnsupportedFormat(t *testing.T) {
	_, err := parseOptions([]string{"--ticker", "AAPL", "--format", "table"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestParseOptions_help(t *testing.T) {
	var usage bytes.Buffer
	prev := osStderr
	osStderr = func() io.Writer { return &usage }
	defer func() { osStderr = prev }()

	_, err := parseOptions([]string{"-h"})
	require.ErrorIs(t, err, flag.ErrHelp)

	out := usage.String()
	assert.Contains(t, out, "confluence-test --ticker TICKER")
	assert.Contains(t, out, "POLYGON_API_KEY")
}

func TestParseOptions_rejectsJsonFalse(t *testing.T) {
	_, err := parseOptions([]string{"--ticker", "TSLA", "--json=false"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--json=false is not supported")
}
