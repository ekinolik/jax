package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestComputeMarket_zeroInputs(t *testing.T) {
	sig := signals.ComputeMarket(signals.MarketInput{})
	assert.Equal(t, confluence.SignalNeutral, sig.Status)
	assert.Equal(t, 0.0, sig.Score)
}

func TestComputeSector_zeroInputs(t *testing.T) {
	sig := signals.ComputeSector(signals.SectorInput{SectorETF: "SMH"})
	assert.Equal(t, confluence.SignalNeutral, sig.Status)
	assert.Equal(t, 0.0, sig.Score)
}
