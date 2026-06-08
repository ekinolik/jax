package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShutdownSequence(t *testing.T) {
	assert.Equal(t, []string{
		"processor",
		"stream-hub",
		"grpc",
		"scheduler",
		"debug-http",
	}, shutdownSequence())
}
