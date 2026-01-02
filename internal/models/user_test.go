package models

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestUserDisplayName_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		user := &User{
			ID:        rapid.Int64().Draw(t, "id"),
			FirstName: rapid.String().Draw(t, "firstName"),
			LastName:  rapid.String().Draw(t, "lastName"),
			Username:  rapid.String().Draw(t, "username"),
		}

		result := user.DisplayName()

		idStr := fmt.Sprintf("[%d]", user.ID)
		if !strings.Contains(result, idStr) {
			t.Fatalf("DisplayName must always contain user_id: got %q, expected to contain %q", result, idStr)
		}

		if user.FirstName != "" && !strings.Contains(result, user.FirstName) {
			t.Fatalf("DisplayName must contain first_name when non-empty: got %q", result)
		}

		if user.LastName != "" && !strings.Contains(result, user.LastName) {
			t.Fatalf("DisplayName must contain last_name when non-empty: got %q", result)
		}

		if user.Username != "" {
			usernameWithAt := "@" + user.Username
			if !strings.Contains(result, usernameWithAt) {
				t.Fatalf("DisplayName must contain @username when non-empty: got %q", result)
			}
		}
	})
}
