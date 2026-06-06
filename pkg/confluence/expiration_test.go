package confluence_test

import (
	"testing"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMonthlyOPEX(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{
			name: "third friday march 2024",
			date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "weekly friday not opex",
			date: time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "non friday",
			date: time.Date(2024, 3, 13, 0, 0, 0, 0, time.UTC),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, confluence.IsMonthlyOPEX(tt.date))
		})
	}
}

func TestResolveExpirationsSingle(t *testing.T) {
	today := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC) // Monday
	dates := []time.Time{
		time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}

	got := confluence.ResolveExpirations(dates, today, false)
	require.Len(t, got, 1)
	assert.Equal(t, "2024-03-11", got[0].Format("2006-01-02"))
}

func TestResolveExpirationsDualSPY(t *testing.T) {
	today := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC) // Monday
	dates := []time.Time{
		time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 12, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC),
	}

	got := confluence.ResolveExpirations(dates, today, true)
	require.Len(t, got, 2)
	assert.Equal(t, "2024-03-11", got[0].Format("2006-01-02"))
	assert.Equal(t, "2024-03-15", got[1].Format("2006-01-02"))
}

func TestResolveExpirationsDualWhenSoonestIsFriday(t *testing.T) {
	today := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC) // OPEX Friday
	dates := []time.Time{
		time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC),
	}

	got := confluence.ResolveExpirations(dates, today, true)
	require.Len(t, got, 2)
	assert.Equal(t, "2024-03-15", got[0].Format("2006-01-02"))
	assert.Equal(t, "2024-03-22", got[1].Format("2006-01-02"))
}

func TestExpiryWeightFor(t *testing.T) {
	monthly := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	weekly := time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, float32(1.0), confluence.ExpiryWeightFor(monthly, 1.0, 0.65))
	assert.Equal(t, float32(0.65), confluence.ExpiryWeightFor(weekly, 1.0, 0.65))
}
