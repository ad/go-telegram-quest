package services

import (
	"fmt"
	"regexp"
	"testing"

	"pgregory.net/rapid"
)

func TestProperty1_PackNameFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000000).Draw(rt, "userID")
		botUsername := rapid.StringMatching(`[a-z][a-z0-9_]{4,30}bot`).Draw(rt, "botUsername")

		service := &StickerService{
			botUsername: botUsername,
		}

		packName := service.GetPackName(userID)

		expectedPattern := fmt.Sprintf(`^achievements_%d_by_%s$`, userID, botUsername)
		matched, err := regexp.MatchString(expectedPattern, packName)
		if err != nil {
			rt.Fatalf("Regex error: %v", err)
		}
		if !matched {
			rt.Fatalf("Pack name %q does not match expected pattern achievements_%d_by_%s", packName, userID, botUsername)
		}
	})
}

func TestProperty2_PackLinkFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1000000000).Draw(rt, "userID")
		botUsername := rapid.StringMatching(`[a-z][a-z0-9_]{4,30}bot`).Draw(rt, "botUsername")

		service := &StickerService{
			botUsername: botUsername,
		}

		packLink := service.GetPackLink(userID)
		packName := service.GetPackName(userID)

		expectedLink := fmt.Sprintf("https://t.me/addstickers/%s", packName)
		if packLink != expectedLink {
			rt.Fatalf("Pack link %q does not match expected %q", packLink, expectedLink)
		}

		linkPattern := `^https://t\.me/addstickers/achievements_\d+_by_[a-z][a-z0-9_]+bot$`
		matched, err := regexp.MatchString(linkPattern, packLink)
		if err != nil {
			rt.Fatalf("Regex error: %v", err)
		}
		if !matched {
			rt.Fatalf("Pack link %q does not match expected URL pattern", packLink)
		}
	})
}

func TestStickerAssetsEmbedded(t *testing.T) {
	expectedStickers := []string{
		"pioneer", "second_place", "third_place",
		"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25",
	}

	service := &StickerService{}

	for _, key := range expectedStickers {
		if !service.stickerExists(key) {
			t.Errorf("Expected sticker %s to be embedded", key)
		}
	}
}

func TestReadEmbeddedSticker(t *testing.T) {
	service := &StickerService{}

	data, err := service.readStickerFile("pioneer")
	if err != nil {
		t.Fatalf("Failed to read embedded sticker: %v", err)
	}

	if len(data) == 0 {
		t.Error("Sticker data should not be empty")
	}

	if len(data) < 1000 {
		t.Errorf("Sticker data seems too small: %d bytes", len(data))
	}
}

func TestProperty7_StickerEmojiCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		manualAchievements := []string{"veteran", "activity", "wow"}
		expectedEmojis := map[string]string{
			"veteran":  "🛡️",
			"activity": "🪩",
			"wow":      "💎",
		}

		achievementKey := rapid.SampledFrom(manualAchievements).Draw(rt, "achievementKey")

		service := &StickerService{}
		actualEmoji := service.getAchievementEmoji(achievementKey)
		expectedEmoji := expectedEmojis[achievementKey]

		if actualEmoji != expectedEmoji {
			rt.Fatalf("For achievement %s, expected emoji %s but got %s", achievementKey, expectedEmoji, actualEmoji)
		}
	})
}
