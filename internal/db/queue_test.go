package db

import (
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

func TestDBQueueRetry_Property(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	queue := NewDBQueueForTest(db)
	defer queue.Close()

	rapid.Check(t, func(t *rapid.T) {
		failUntil := rapid.IntRange(0, 4).Draw(t, "failUntil")
		expectedData := rapid.Int().Draw(t, "expectedData")

		var attempts int32

		task := func(_ *sql.DB) (interface{}, error) {
			attempt := int(atomic.AddInt32(&attempts, 1))
			if attempt <= failUntil {
				return nil, errors.New("simulated failure")
			}
			return expectedData, nil
		}

		result, err := queue.Execute(task)

		actualAttempts := int(atomic.LoadInt32(&attempts))

		if failUntil >= 3 {
			if err == nil {
				t.Fatalf("expected error after 3 retries, got nil")
			}
			if actualAttempts != 3 {
				t.Fatalf("expected exactly 3 attempts, got %d", actualAttempts)
			}
		} else {
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if result != expectedData {
				t.Fatalf("expected data %v, got %v", expectedData, result)
			}
			expectedAttempts := failUntil + 1
			if actualAttempts != expectedAttempts {
				t.Fatalf("expected %d attempts, got %d", expectedAttempts, actualAttempts)
			}
		}
	})
}
