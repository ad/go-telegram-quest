package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type StickerService struct {
	bot             *bot.Bot
	stickerPackRepo *db.StickerPackRepository
	botUsername     string
	botToken        string
}

func NewStickerService(b *bot.Bot, repo *db.StickerPackRepository, botUsername, botToken string) *StickerService {
	return &StickerService{
		bot:             b,
		stickerPackRepo: repo,
		botUsername:     botUsername,
		botToken:        botToken,
	}
}

func (s *StickerService) GetPackName(userID int64) string {
	return fmt.Sprintf("achievements_%d_by_%s", userID, s.botUsername)
}

func (s *StickerService) GetPackLink(userID int64) string {
	return fmt.Sprintf("https://t.me/addstickers/%s", s.GetPackName(userID))
}

func (s *StickerService) HasStickerPack(userID int64) (bool, error) {
	return s.stickerPackRepo.Exists(userID)
}

func (s *StickerService) getStickerAssetPath(achievementKey string) string {
	return "assets/" + achievementKey + ".webp"
}

func (s *StickerService) readStickerFile(achievementKey string) ([]byte, error) {
	assetPath := s.getStickerAssetPath(achievementKey)
	log.Printf("[STICKER_SERVICE] Reading sticker file: %s", assetPath)

	data, err := stickerAssets.ReadFile(assetPath)
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to read embedded file %s: %v", assetPath, err)
		return nil, err
	}

	log.Printf("[STICKER_SERVICE] Successfully read %d bytes from %s", len(data), assetPath)
	return data, nil
}

func (s *StickerService) stickerExists(achievementKey string) bool {
	assetPath := s.getStickerAssetPath(achievementKey)
	_, err := stickerAssets.ReadFile(assetPath)
	return err == nil
}

func (s *StickerService) createStickerPack(ctx context.Context, userID int64, achievementKey, emoji string) (string, error) {
	packName := s.GetPackName(userID)

	fileContent, err := s.readStickerFile(achievementKey)
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to read sticker file %s: %v", achievementKey, err)
		return "", fmt.Errorf("failed to read sticker file: %w", err)
	}

	inputSticker := tgmodels.InputSticker{
		Sticker:           "attach://sticker.webp",
		Format:            "static",
		EmojiList:         []string{emoji},
		StickerAttachment: bytes.NewBuffer(fileContent),
	}

	params := &bot.CreateNewStickerSetParams{
		UserID:   userID,
		Name:     packName,
		Title:    "Quest Achievements",
		Stickers: []tgmodels.InputSticker{inputSticker},
	}

	_, err = s.bot.CreateNewStickerSet(ctx, params)
	if err != nil {
		if strings.Contains(err.Error(), "STICKER_SET_NAME_OCCUPIED") {
			log.Printf("[STICKER_SERVICE] Sticker pack %s already exists, updating DB", packName)
			if dbErr := s.stickerPackRepo.Create(userID, packName); dbErr != nil {
				log.Printf("[STICKER_SERVICE] Failed to update DB for existing pack: %v", dbErr)
			}
			return s.getLastStickerFileID(ctx, packName), nil
		}
		log.Printf("[STICKER_SERVICE] Failed to create sticker pack: %v", err)
		return "", fmt.Errorf("failed to create sticker pack: %w", err)
	}

	if err := s.stickerPackRepo.Create(userID, packName); err != nil {
		log.Printf("[STICKER_SERVICE] Failed to save sticker pack to DB: %v", err)
		return "", fmt.Errorf("failed to save sticker pack: %w", err)
	}

	log.Printf("[STICKER_SERVICE] Created sticker pack %s for user %d", packName, userID)
	return s.getLastStickerFileID(ctx, packName), nil
}

func (s *StickerService) addStickerToSet(ctx context.Context, userID int64, achievementKey, emoji string) (string, error) {
	packName := s.GetPackName(userID)

	exists, existingFileID := s.stickerExistsInPack(ctx, packName, achievementKey)
	if exists {
		return existingFileID, nil
	}

	fileContent, err := s.readStickerFile(achievementKey)
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to read sticker file %s: %v", achievementKey, err)
		return "", fmt.Errorf("failed to read sticker file: %w", err)
	}

	err = s.addStickerToSetRaw(ctx, userID, packName, fileContent, emoji)
	if err != nil {
		if strings.Contains(err.Error(), "STICKER_EMOJI_INVALID") ||
			strings.Contains(err.Error(), "STICKERS_TOO_MUCH") {
			log.Printf("[STICKER_SERVICE] Cannot add sticker: %v", err)
			return "", nil
		}
		log.Printf("[STICKER_SERVICE] Failed to add sticker to set: %v", err)
		return "", fmt.Errorf("failed to add sticker: %w", err)
	}

	log.Printf("[STICKER_SERVICE] Added sticker %s to pack %s", achievementKey, packName)
	return s.getLastStickerFileID(ctx, packName), nil
}

func (s *StickerService) addStickerToSetRaw(ctx context.Context, userID int64, packName string, fileContent []byte, emoji string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("user_id", fmt.Sprintf("%d", userID))
	writer.WriteField("name", packName)

	stickerJSON := fmt.Sprintf(`{"sticker": "attach://file", "format": "static", "emoji_list": ["%s"]}`, emoji)
	writer.WriteField("sticker", stickerJSON)

	part, err := writer.CreateFormFile("file", "sticker.webp")
	if err != nil {
		return err
	}
	part.Write(fileContent)
	writer.Close()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/addStickerToSet", s.botToken)

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	json.Unmarshal(respBody, &result)

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	return nil
}

func (s *StickerService) EnsureStickerPack(ctx context.Context, userID int64, achievementKey, emoji string) (string, error) {
	log.Printf("[STICKER_SERVICE] EnsureStickerPack called for user %d, achievement %s, emoji %s", userID, achievementKey, emoji)

	if !s.stickerExists(achievementKey) {
		log.Printf("[STICKER_SERVICE] Sticker file not found: %s", achievementKey)
		return "", nil
	}

	exists, err := s.HasStickerPack(userID)
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to check sticker pack existence: %v", err)
		return "", nil
	}

	log.Printf("[STICKER_SERVICE] User %d sticker pack exists: %v", userID, exists)

	var stickerFileID string
	if !exists {
		log.Printf("[STICKER_SERVICE] Creating new sticker pack for user %d", userID)
		stickerFileID, err = s.createStickerPack(ctx, userID, achievementKey, emoji)
		if err != nil {
			log.Printf("[STICKER_SERVICE] Failed to create sticker pack: %v", err)
			return "", nil
		}
	} else {
		log.Printf("[STICKER_SERVICE] Adding sticker to existing pack for user %d", userID)
		stickerFileID, err = s.addStickerToSet(ctx, userID, achievementKey, emoji)
		if err != nil {
			log.Printf("[STICKER_SERVICE] Failed to add sticker to set: %v", err)
			return "", nil
		}
	}

	log.Printf("[STICKER_SERVICE] EnsureStickerPack completed, fileID: %s", stickerFileID)
	return stickerFileID, nil
}

func (s *StickerService) getLastStickerFileID(ctx context.Context, packName string) string {
	stickerSet, err := s.bot.GetStickerSet(ctx, &bot.GetStickerSetParams{Name: packName})
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to get sticker set %s: %v", packName, err)
		return ""
	}
	if len(stickerSet.Stickers) == 0 {
		return ""
	}
	return stickerSet.Stickers[len(stickerSet.Stickers)-1].FileID
}

func (s *StickerService) stickerExistsInPack(ctx context.Context, packName, achievementKey string) (bool, string) {
	stickerSet, err := s.bot.GetStickerSet(ctx, &bot.GetStickerSetParams{Name: packName})
	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to get sticker set %s: %v", packName, err)
		return false, ""
	}

	// Проверяем по emoji - каждое достижение имеет уникальный emoji
	targetEmoji := s.getAchievementEmoji(achievementKey)
	for _, sticker := range stickerSet.Stickers {
		if len(sticker.Emoji) > 0 && sticker.Emoji == targetEmoji {
			log.Printf("[STICKER_SERVICE] Sticker for achievement %s already exists in pack %s", achievementKey, packName)
			return true, sticker.FileID
		}
	}

	return false, ""
}

func (s *StickerService) getAchievementEmoji(achievementKey string) string {
	// Используем тот же маппинг что и в AchievementNotifier
	achievementEmojis := map[string]string{
		"pioneer":         "🥇",
		"second_place":    "🥈",
		"third_place":     "🥉",
		"beginner_5":      "🌱",
		"experienced_10":  "🌿",
		"advanced_15":     "🌳",
		"expert_20":       "🏅",
		"master_25":       "🎓",
		"winner":          "🏆",
		"perfect_path":    "✨",
		"self_sufficient": "💪",
		"lightning":       "⚡",
		"rocket":          "🚀",
		"cheater":         "🃏",
		"photographer":    "📸",
		"paparazzi":       "📷",
		"bullseye":        "🎯",
		"secret_agent":    "🕵️",
		"curious":         "🤔",
		"fan":             "❤️",
		"hint_5":          "💡",
		"hint_10":         "🔍",
		"hint_15":         "🔎",
		"hint_25":         "🧐",
		"hint_master":     "🧙",
		"skeptic":         "🤨",
		"super_collector": "🎁",
		"super_brain":     "🧠",
		"legend":          "👑",
	}

	if emoji, ok := achievementEmojis[achievementKey]; ok {
		return emoji
	}
	return "🏅" // fallback
}

func (s *StickerService) SendSticker(ctx context.Context, chatID int64, stickerFileID string) error {
	if stickerFileID == "" {
		log.Printf("[STICKER_SERVICE] SendSticker called with empty fileID for chat %d", chatID)
		return nil
	}

	// log.Printf("[STICKER_SERVICE] Sending sticker %s to chat %d", stickerFileID, chatID)

	_, err := s.bot.SendSticker(ctx, &bot.SendStickerParams{
		ChatID:  chatID,
		Sticker: &tgmodels.InputFileString{Data: stickerFileID},
	})

	if err != nil {
		log.Printf("[STICKER_SERVICE] Failed to send sticker to chat %d: %v", chatID, err)
	} else {
		log.Printf("[STICKER_SERVICE] Successfully sent sticker to chat %d", chatID)
	}

	return err
}
