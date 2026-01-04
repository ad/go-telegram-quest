package models

import (
	"testing"

	"pgregory.net/rapid"
)

func TestProperty1_HintDataPersistence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		step := &Step{
			ID:        rapid.Int64().Draw(t, "id"),
			StepOrder: rapid.Int().Draw(t, "stepOrder"),
			Text:      rapid.String().Draw(t, "text"),
			HintText:  rapid.String().Draw(t, "hintText"),
			HintImage: rapid.String().Draw(t, "hintImage"),
		}

		hasHint := step.HasHint()
		expectedHasHint := step.HintText != "" || step.HintImage != ""

		if hasHint != expectedHasHint {
			t.Fatalf("HasHint() returned %v, but expected %v for HintText=%q, HintImage=%q",
				hasHint, expectedHasHint, step.HintText, step.HintImage)
		}

		// Test specific cases
		if step.HintText == "" && step.HintImage == "" && hasHint {
			t.Fatalf("HasHint() should return false when both HintText and HintImage are empty")
		}

		if (step.HintText != "" || step.HintImage != "") && !hasHint {
			t.Fatalf("HasHint() should return true when at least one of HintText or HintImage is non-empty")
		}
	})
}
