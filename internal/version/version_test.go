package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLine_defaultVersion(t *testing.T) {
	assert.Equal(t, "jax dev", Line())
}

func TestLine_withVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "0.2.00005"
	assert.Equal(t, "jax 0.2.00005", Line())
}
