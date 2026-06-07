package confluence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugPortFromEnv_loopback(t *testing.T) {
	assert.Equal(t, "127.0.0.1:8081", DebugPortFromEnv("8081"))
	assert.Equal(t, "", DebugPortFromEnv(""))
	assert.Equal(t, "", DebugPortFromEnv("not-a-port"))
}
