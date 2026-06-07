package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestComputeRSI_minuteOversold(t *testing.T) {
	sig := signals.ComputeRSI(signals.RSIInput{RSI: 30, Weight: 0.12})
	assert.Equal(t, "rsi_minute", sig.Name)
	assert.Greater(t, sig.AxisFill, 0.7)
}

func TestComputeRSI_dailySofterCurve(t *testing.T) {
	minute := signals.ComputeRSI(signals.RSIInput{RSI: 32, Weight: 0.12})
	daily := signals.ComputeRSI(signals.RSIInput{RSI: 32, Weight: 0.03, Daily: true})
	assert.Equal(t, "rsi_daily", daily.Name)
	assert.Less(t, daily.AxisFill, minute.AxisFill)
}

func TestComputeRSI_validPassesThrough(t *testing.T) {
	sig := signals.ComputeRSI(signals.RSIInput{RSI: 54.9, Weight: 0.12})
	assert.Equal(t, "rsi_minute", sig.Name)
	assert.Equal(t, "RSI-14 minute 54.9", sig.Detail)
	assert.Greater(t, sig.AxisFill, 0.0)
}

func TestComputeRSI_unavailable(t *testing.T) {
	sig := signals.ComputeRSI(signals.RSIInput{RSI: 0, Weight: 0.12})
	assert.Equal(t, "RSI unavailable", sig.Detail)
	assert.Equal(t, 0.0, sig.AxisFill)
}

func TestComputeRSI_outOfRange_NETRegression(t *testing.T) {
	// Polygon minute RSI for NET returned ~0.08 from synthetic flat bars.
	sig := signals.ComputeRSI(signals.RSIInput{RSI: 0.08122720260347194, Weight: 0.12})
	assert.Equal(t, "RSI out of range", sig.Detail)
	assert.Equal(t, 0.0, sig.AxisFill)
}

func TestComputeRSI_outOfRange_aboveMax(t *testing.T) {
	sig := signals.ComputeRSI(signals.RSIInput{RSI: 99.5, Weight: 0.12})
	assert.Equal(t, "RSI out of range", sig.Detail)
	assert.Equal(t, 0.0, sig.AxisFill)
}
