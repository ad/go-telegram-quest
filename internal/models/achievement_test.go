package models

import (
	"testing"

	"pgregory.net/rapid"
)

func TestProperty1_AchievementDataPersistence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		conditions := AchievementConditions{
			CorrectAnswers:        intPtr(rapid.IntRange(0, 100).Draw(t, "correctAnswers")),
			CompletionTimeMinutes: intPtr(rapid.IntRange(1, 1440).Draw(t, "completionTime")),
			NoErrors:              boolPtr(rapid.Bool().Draw(t, "noErrors")),
			NoHints:               boolPtr(rapid.Bool().Draw(t, "noHints")),
			HintCount:             intPtr(rapid.IntRange(0, 50).Draw(t, "hintCount")),
			ConsecutiveCorrect:    intPtr(rapid.IntRange(0, 50).Draw(t, "consecutiveCorrect")),
			Position:              intPtr(rapid.IntRange(1, 100).Draw(t, "position")),
			PhotoSubmitted:        boolPtr(rapid.Bool().Draw(t, "photoSubmitted")),
			HintOnFirstTask:       boolPtr(rapid.Bool().Draw(t, "hintOnFirstTask")),
			AllHintsUsed:          boolPtr(rapid.Bool().Draw(t, "allHintsUsed")),
			PhotoOnTextTask:       boolPtr(rapid.Bool().Draw(t, "photoOnTextTask")),
			InactiveHours:         intPtr(rapid.IntRange(1, 168).Draw(t, "inactiveHours")),
			PostCompletion:        boolPtr(rapid.Bool().Draw(t, "postCompletion")),
		}

		if rapid.Bool().Draw(t, "hasSpecificAnswer") {
			answer := rapid.String().Draw(t, "specificAnswer")
			conditions.SpecificAnswer = &answer
		}

		if rapid.Bool().Draw(t, "hasRequiredAchievements") {
			count := rapid.IntRange(1, 5).Draw(t, "requiredCount")
			required := make([]string, count)
			for i := 0; i < count; i++ {
				required[i] = rapid.StringMatching(`[a-z_]+`).Draw(t, "requiredAchievement")
			}
			conditions.RequiredAchievements = required
		}

		jsonStr, err := conditions.ToJSON()
		if err != nil {
			t.Fatalf("ToJSON failed: %v", err)
		}

		parsed, err := ParseAchievementConditions(jsonStr)
		if err != nil {
			t.Fatalf("ParseAchievementConditions failed: %v", err)
		}

		if !compareIntPtr(conditions.CorrectAnswers, parsed.CorrectAnswers) {
			t.Fatalf("CorrectAnswers mismatch: %v != %v", ptrVal(conditions.CorrectAnswers), ptrVal(parsed.CorrectAnswers))
		}
		if !compareIntPtr(conditions.CompletionTimeMinutes, parsed.CompletionTimeMinutes) {
			t.Fatalf("CompletionTimeMinutes mismatch")
		}
		if !compareBoolPtr(conditions.NoErrors, parsed.NoErrors) {
			t.Fatalf("NoErrors mismatch")
		}
		if !compareBoolPtr(conditions.NoHints, parsed.NoHints) {
			t.Fatalf("NoHints mismatch")
		}
		if !compareIntPtr(conditions.HintCount, parsed.HintCount) {
			t.Fatalf("HintCount mismatch")
		}
		if !compareStringPtr(conditions.SpecificAnswer, parsed.SpecificAnswer) {
			t.Fatalf("SpecificAnswer mismatch")
		}
		if !compareBoolPtr(conditions.PhotoSubmitted, parsed.PhotoSubmitted) {
			t.Fatalf("PhotoSubmitted mismatch")
		}
		if !compareIntPtr(conditions.ConsecutiveCorrect, parsed.ConsecutiveCorrect) {
			t.Fatalf("ConsecutiveCorrect mismatch")
		}
		if !compareIntPtr(conditions.Position, parsed.Position) {
			t.Fatalf("Position mismatch")
		}
		if !compareStringSlice(conditions.RequiredAchievements, parsed.RequiredAchievements) {
			t.Fatalf("RequiredAchievements mismatch")
		}
		if !compareBoolPtr(conditions.HintOnFirstTask, parsed.HintOnFirstTask) {
			t.Fatalf("HintOnFirstTask mismatch")
		}
		if !compareBoolPtr(conditions.AllHintsUsed, parsed.AllHintsUsed) {
			t.Fatalf("AllHintsUsed mismatch")
		}
		if !compareBoolPtr(conditions.PhotoOnTextTask, parsed.PhotoOnTextTask) {
			t.Fatalf("PhotoOnTextTask mismatch")
		}
		if !compareIntPtr(conditions.InactiveHours, parsed.InactiveHours) {
			t.Fatalf("InactiveHours mismatch")
		}
		if !compareBoolPtr(conditions.PostCompletion, parsed.PostCompletion) {
			t.Fatalf("PostCompletion mismatch")
		}
	})
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }
func ptrVal(p *int) string {
	if p == nil {
		return "nil"
	}
	return string(rune(*p))
}

func compareIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func compareBoolPtr(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func compareStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func compareStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
