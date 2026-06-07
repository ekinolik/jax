package polygon_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ekinolik/jax/internal/polygon"
	"github.com/massive-com/client-go/v2/rest/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRetryableAPIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &models.ErrorResponse{StatusCode: 429}, true},
		{"500", &models.ErrorResponse{StatusCode: 500}, true},
		{"404", &models.ErrorResponse{StatusCode: 404}, false},
		{"message429", errors.New("bad status with code '429'"), true},
		{"permanent", errors.New("invalid ticker"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, polygon.IsRetryableAPIError(tt.err))
		})
	}
}

func TestWithRetry_succeedsAfterTransientFailure(t *testing.T) {
	attempts := 0
	err := polygon.WithRetry(context.Background(), polygon.RetryConfig{
		MaxRetries:  3,
		BaseDelayMs: 1,
	}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return &models.ErrorResponse{StatusCode: 429, BaseResponse: models.BaseResponse{ErrorMessage: "rate limited"}}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestWithRetry_doesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	err := polygon.WithRetry(context.Background(), polygon.RetryConfig{
		MaxRetries:  3,
		BaseDelayMs: 1,
	}, func(context.Context) error {
		attempts++
		return &models.ErrorResponse{StatusCode: 404, BaseResponse: models.BaseResponse{ErrorMessage: "not found"}}
	})
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestWithRetry_respectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := polygon.WithRetry(ctx, polygon.DefaultRetryConfig(), func(context.Context) error {
		return &models.ErrorResponse{StatusCode: 503}
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
