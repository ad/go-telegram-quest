package db

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

var stickerPackTestDBCounter int64

func setupStickerPackTestDB(t *testing.T) (*sql.DB, *StickerPackRepository, *UserRepository) {
	counter := atomic.AddInt64(&stickerPackTestDBCounter, 1)
	dsn := fmt.Sprintf("file:sticker_pack_test_%d?mode=memory&cache=shared", counter)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}

	queue := NewDBQueue(db)
	stickerPackRepo := NewStickerPackRepository(queue)
	userRepo := NewUserRepository(queue)

	return db, stickerPackRepo, userRepo
}

func TestProperty5_RepositoryRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db, stickerPackRepo, userRepo := setupStickerPackTestDB(t)
		defer db.Close()

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		createTestUser(t, userRepo, userID)

		packName := rapid.StringMatching(`quest_[0-9]+_by_[a-z_]+bot`).Draw(rt, "packName")

		err := stickerPackRepo.Create(userID, packName)
		if err != nil {
			rt.Fatalf("Create failed: %v", err)
		}

		pack, err := stickerPackRepo.GetByUserID(userID)
		if err != nil {
			rt.Fatalf("GetByUserID failed: %v", err)
		}

		if pack.UserID != userID {
			rt.Fatalf("Expected UserID %d, got %d", userID, pack.UserID)
		}

		if pack.PackName != packName {
			rt.Fatalf("Expected PackName %s, got %s", packName, pack.PackName)
		}
	})
}

func TestProperty6_ExistsConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db, stickerPackRepo, userRepo := setupStickerPackTestDB(t)
		defer db.Close()

		userID := rapid.Int64Range(1, 1000000).Draw(rt, "userID")
		createTestUser(t, userRepo, userID)

		exists, err := stickerPackRepo.Exists(userID)
		if err != nil {
			rt.Fatalf("Exists failed: %v", err)
		}
		if exists {
			rt.Fatal("Exists should return false before Create")
		}

		packName := rapid.StringMatching(`quest_[0-9]+_by_[a-z_]+bot`).Draw(rt, "packName")
		err = stickerPackRepo.Create(userID, packName)
		if err != nil {
			rt.Fatalf("Create failed: %v", err)
		}

		exists, err = stickerPackRepo.Exists(userID)
		if err != nil {
			rt.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			rt.Fatal("Exists should return true after Create")
		}
	})
}

func TestStickerPackRepository_GetByUserID_NotFound(t *testing.T) {
	db, stickerPackRepo, _ := setupStickerPackTestDB(t)
	defer db.Close()

	_, err := stickerPackRepo.GetByUserID(999999)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}
