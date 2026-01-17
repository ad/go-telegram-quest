package services

import (
	"regexp"
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

func TestProperty8_NestedHTMLTagValidity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate test statistics with various combinations of formatting
		stats := &UserStatistics{
			TotalAnswers:        rapid.IntRange(0, 1000).Draw(rt, "totalAnswers"),
			ApprovedSteps:       rapid.IntRange(0, 100).Draw(rt, "approvedSteps"),
			Accuracy:            rapid.IntRange(0, 100).Draw(rt, "accuracy"),
			LeaderboardPosition: rapid.IntRange(1, 1000).Draw(rt, "position"),
			TotalUsers:          rapid.IntRange(1, 1000).Draw(rt, "totalUsers"),
			RegistrationDate:    time.Now().Add(-time.Duration(rapid.IntRange(1, 365*24).Draw(rt, "regDays")) * time.Hour),
		}

		// Add optional fields randomly
		if rapid.Bool().Draw(rt, "hasAvgResponseTime") {
			avgTime := time.Duration(rapid.IntRange(1, 3600).Draw(rt, "avgSeconds")) * time.Second
			stats.AverageResponseTime = &avgTime
		}

		if rapid.Bool().Draw(rt, "hasTimeOnCurrentStep") {
			currentTime := time.Duration(rapid.IntRange(1, 3600).Draw(rt, "currentSeconds")) * time.Second
			stats.TimeOnCurrentStep = &currentTime
		}

		if rapid.Bool().Draw(rt, "hasFirstAnswerTime") {
			firstTime := time.Now().Add(-time.Duration(rapid.IntRange(1, 100).Draw(rt, "firstHours")) * time.Hour)
			stats.FirstAnswerTime = &firstTime
		}

		if rapid.Bool().Draw(rt, "hasLastAnswerTime") {
			lastTime := time.Now().Add(-time.Duration(rapid.IntRange(1, 10).Draw(rt, "lastHours")) * time.Hour)
			stats.LastAnswerTime = &lastTime
		}

		if rapid.Bool().Draw(rt, "hasCompletionTime") {
			completionTime := time.Duration(rapid.IntRange(1, 24*3600).Draw(rt, "completionSeconds")) * time.Second
			stats.CompletionTime = &completionTime
		}

		// Generate step attempts
		numAttempts := rapid.IntRange(0, 5).Draw(rt, "numAttempts")
		for i := 0; i < numAttempts; i++ {
			stats.StepAttempts = append(stats.StepAttempts, StepAttempt{
				StepOrder: rapid.IntRange(1, 50).Draw(rt, "stepOrder"),
				Attempts:  rapid.IntRange(2, 10).Draw(rt, "attempts"),
			})
		}

		isCompleted := rapid.Bool().Draw(rt, "isCompleted")
		result := FormatUserStatistics(stats, isCompleted)

		// Verify HTML tags are properly nested and closed
		if !isValidHTML(result) {
			rt.Errorf("FormatUserStatistics produced invalid HTML: %s", result)
		}

		// Verify bold tags are properly used
		if !strings.Contains(result, "<b>") || !strings.Contains(result, "</b>") {
			rt.Errorf("FormatUserStatistics should contain properly formatted bold tags")
		}

		// Verify no markdown formatting is present
		if strings.Contains(result, "*") && !strings.Contains(result, "•") {
			rt.Errorf("FormatUserStatistics should not contain markdown asterisks: %s", result)
		}

		// Verify content is properly escaped (though UserStatistics doesn't contain user content)
		if strings.Contains(result, "<script>") || strings.Contains(result, "&lt;script&gt;") {
			rt.Errorf("FormatUserStatistics should not contain unescaped script tags")
		}
	})
}

// isValidHTML checks if HTML tags are properly nested and closed
func isValidHTML(content string) bool {
	// Simple validation for basic HTML tags used in formatting
	tagPattern := regexp.MustCompile(`<(/?)([a-zA-Z]+)>`)
	matches := tagPattern.FindAllStringSubmatch(content, -1)

	var stack []string
	for _, match := range matches {
		isClosing := match[1] == "/"
		tagName := match[2]

		if isClosing {
			if len(stack) == 0 || stack[len(stack)-1] != tagName {
				return false // Mismatched closing tag
			}
			stack = stack[:len(stack)-1] // Pop from stack
		} else {
			stack = append(stack, tagName) // Push to stack
		}
	}

	return len(stack) == 0 // All tags should be closed
}
