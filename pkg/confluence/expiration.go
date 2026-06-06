package confluence

import (
	"sort"
	"time"
)

const (
	DefaultMonthlyExpiryWeight = 1.0
	DefaultWeeklyExpiryWeight  = 0.65
)

// IsMonthlyOPEX reports whether expiration is the standard 3rd-Friday monthly OPEX.
func IsMonthlyOPEX(expiration time.Time) bool {
	expiration = DateOnly(expiration)
	if expiration.Weekday() != time.Friday {
		return false
	}
	third := thirdFriday(expiration.Year(), expiration.Month())
	return sameDate(expiration, third)
}

// DateOnly truncates a time to UTC midnight for calendar-day comparisons.
func DateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ExpiryWeightFor returns the strength multiplier for an expiration date.
func ExpiryWeightFor(expiration time.Time, monthlyWeight, weeklyWeight float32) float32 {
	if IsMonthlyOPEX(expiration) {
		return monthlyWeight
	}
	return weeklyWeight
}

// ResolveExpirations picks soonest expiration(s) on or after today.
// today should be the market calendar date (America/New_York).
func ResolveExpirations(dates []time.Time, today time.Time, dualExpiration bool) []time.Time {
	today = DateOnly(today)

	unique := make(map[int64]time.Time)
	for _, d := range dates {
		d = DateOnly(d)
		if d.Before(today) {
			continue
		}
		unique[d.Unix()] = d
	}
	if len(unique) == 0 {
		return nil
	}

	sorted := make([]time.Time, 0, len(unique))
	for _, d := range unique {
		sorted = append(sorted, d)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before(sorted[j])
	})

	soonest := sorted[0]
	result := []time.Time{soonest}
	if !dualExpiration {
		return result
	}

	if weekly := findSoonestWeekly(sorted, soonest); weekly != nil && !sameDate(*weekly, soonest) {
		result = append(result, *weekly)
	}
	return result
}

func findSoonestWeekly(dates []time.Time, soonest time.Time) *time.Time {
	var fridays []time.Time
	for _, d := range dates {
		if d.Weekday() == time.Friday {
			fridays = append(fridays, d)
		}
	}
	if len(fridays) == 0 {
		return nil
	}
	sort.Slice(fridays, func(i, j int) bool {
		return fridays[i].Before(fridays[j])
	})

	if soonest.Weekday() == time.Friday {
		for _, f := range fridays {
			if f.After(soonest) {
				return &f
			}
		}
		return nil
	}

	for _, f := range fridays {
		if !f.Before(soonest) {
			return &f
		}
	}
	return nil
}

func thirdFriday(year int, month time.Month) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(time.Friday) - int(first.Weekday()) + 7) % 7
	firstFriday := first.AddDate(0, 0, offset)
	return firstFriday.AddDate(0, 0, 14)
}

func sameDate(a, b time.Time) bool {
	return DateOnly(a).Equal(DateOnly(b))
}
