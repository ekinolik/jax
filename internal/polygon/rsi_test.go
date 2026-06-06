package polygon

import "testing"

func TestValidRSI(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "mid range", value: 58.9, want: true},
		{name: "oversold edge", value: 1.0, want: true},
		{name: "overbought edge", value: 99.0, want: true},
		{name: "zero unavailable", value: 0, want: false},
		{name: "NET bad minute", value: 0.08122720260347194, want: false},
		{name: "below min", value: 0.1, want: false},
		{name: "high but valid", value: 96.21, want: true},
		{name: "at 100", value: 100, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidRSI(tt.value); got != tt.want {
				t.Fatalf("ValidRSI(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
