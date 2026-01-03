package services

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestProperty6_DurationFormatting(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		seconds := rapid.Uint64Range(0, 365*24*3600).Draw(rt, "seconds")
		duration := time.Duration(seconds) * time.Second

		result := FormatDurationRussian(duration)

		if result == "" {
			rt.Errorf("FormatDurationRussian returned empty string for duration %v", duration)
		}

		validUnits := []string{"д", "ч", "м", "с"}
		hasValidUnit := false
		for _, unit := range validUnits {
			if strings.Contains(result, unit) {
				hasValidUnit = true
				break
			}
		}

		if !hasValidUnit {
			rt.Errorf("FormatDurationRussian result '%s' does not contain any valid time unit", result)
		}

		if duration == 0 && result != "0с" {
			rt.Errorf("FormatDurationRussian for zero duration should return '0с', got '%s'", result)
		}
	})
}
