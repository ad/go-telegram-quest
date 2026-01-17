package services

import (
	"context"
	"fmt"
	"html"
	"log"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
	"github.com/go-telegram/bot"
)

type AchievementNotifier struct {
	bot             *bot.Bot
	achievementRepo *db.AchievementRepository
	msgManager      *MessageManager
	stickerService  *StickerService
}

func NewAchievementNotifier(
	b *bot.Bot,
	achievementRepo *db.AchievementRepository,
	msgManager *MessageManager,
	stickerService *StickerService,
) *AchievementNotifier {
	return &AchievementNotifier{
		bot:             b,
		achievementRepo: achievementRepo,
		msgManager:      msgManager,
		stickerService:  stickerService,
	}
}

var categoryEmojis = map[models.AchievementCategory]string{
	models.CategoryProgress:   "📈",
	models.CategoryCompletion: "🏆",
	models.CategorySpecial:    "⭐",
	models.CategoryHints:      "💡",
	models.CategoryComposite:  "👑",
	models.CategoryUnique:     "🎖️",
}

var achievementEmojis = map[string]string{
	"pioneer":         "🔥",
	"second_place":    "🌟",
	"third_place":     "💫",
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
	"hint_30":         "🔮",
	"hint_master":     "🧙",
	"skeptic":         "🤨",
	"super_collector": "🎁",
	"super_brain":     "🧠",
	"legend":          "👑",
	"winner_1":        "🥇",
	"winner_2":        "🥈",
	"winner_3":        "🥉",
	"restart":         "🔄",
	"writer":          "✍️",
	"veteran":         "🛡️",
	"activity":        "🪩",
	"wow":             "💎",
	"asterisk":        "⭐",
	"unseen":          "👁️",
	"voice":           "📢",
}

func (n *AchievementNotifier) GetAchievementEmoji(achievement *models.Achievement) string {
	if emoji, ok := achievementEmojis[achievement.Key]; ok {
		return emoji
	}
	if emoji, ok := categoryEmojis[achievement.Category]; ok {
		return emoji
	}
	return "🏅"
}

func (n *AchievementNotifier) FormatNotification(achievement *models.Achievement) string {
	emoji := n.GetAchievementEmoji(achievement)
	return fmt.Sprintf(
		"🎉 <b>Вы получили достижение:</b> %s <code>%s</code>\n\n<i>%s</i>",
		emoji,
		html.EscapeString(achievement.Name),
		html.EscapeString(achievement.Description),
	)
}

func (n *AchievementNotifier) FormatStickerPackMessage(userID int64) string {
	if n.stickerService == nil {
		return ""
	}

	hasPack, err := n.stickerService.HasStickerPack(userID)
	if err != nil {
		log.Printf("[ACHIEVEMENT_NOTIFIER] Failed to check sticker pack for user %d: %v", userID, err)
		return ""
	}

	if !hasPack {
		return ""
	}

	stickerLink := n.stickerService.GetPackLink(userID)
	return fmt.Sprintf("🎨 Вот стикер-пак со всеми вашими достижениями, вы можете добавить его и поделиться уникальными стикерами: %s", stickerLink)
}

func (n *AchievementNotifier) NotifyAchievement(ctx context.Context, userID int64, achievementKey string) error {
	achievement, err := n.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		return fmt.Errorf("failed to get achievement %s: %w", achievementKey, err)
	}

	var stickerFileID string
	if n.stickerService != nil {
		emoji := n.GetAchievementEmoji(achievement)
		stickerFileID, err = n.stickerService.EnsureStickerPack(ctx, userID, achievementKey, emoji)
		if err != nil {
			log.Printf("[ACHIEVEMENT_NOTIFIER] Failed to ensure sticker pack for user %d: %v", userID, err)
		}
	}

	message := n.FormatNotification(achievement)
	if err := n.sendNotification(ctx, userID, message); err != nil {
		return err
	}

	if stickerFileID != "" && n.stickerService != nil {
		// log.Printf("[ACHIEVEMENT_NOTIFIER] Sending sticker %s to user %d", stickerFileID, userID)
		if err := n.stickerService.SendSticker(ctx, userID, stickerFileID); err != nil {
			log.Printf("[ACHIEVEMENT_NOTIFIER] Failed to send sticker to user %d: %v", userID, err)
		} else {
			// log.Printf("[ACHIEVEMENT_NOTIFIER] Successfully sent sticker to user %d", userID)
		}
	} else {
		log.Printf("[ACHIEVEMENT_NOTIFIER] Not sending sticker: fileID='%s', stickerService=%v", stickerFileID, n.stickerService != nil)
	}

	return nil
}

func (n *AchievementNotifier) NotifyAchievements(ctx context.Context, userID int64, achievementKeys []string) error {
	if len(achievementKeys) == 0 {
		return nil
	}

	for _, key := range achievementKeys {
		if err := n.NotifyAchievement(ctx, userID, key); err != nil {
			log.Printf("[ACHIEVEMENT_NOTIFIER] Error notifying user %d about achievement %s: %v", userID, key, err)
		}
	}

	return nil
}

func (n *AchievementNotifier) sendNotification(ctx context.Context, userID int64, message string) error {
	params := &bot.SendMessageParams{
		ChatID: userID,
		Text:   message,
	}

	_, err := n.msgManager.SendWithRetryAndEffect(ctx, params, "5104841245755180586")
	if err != nil {
		log.Printf("[ACHIEVEMENT_NOTIFIER] Failed to send notification to user %d: %v", userID, err)
		return err
	}

	return nil
}

type AchievementNotification struct {
	UserID         int64
	AchievementKey string
	Achievement    *models.Achievement
	Message        string
}

func (n *AchievementNotifier) PrepareNotifications(achievementKeys []string) ([]*AchievementNotification, error) {
	var notifications []*AchievementNotification

	for _, key := range achievementKeys {
		achievement, err := n.achievementRepo.GetByKey(key)
		if err != nil {
			log.Printf("[ACHIEVEMENT_NOTIFIER] Failed to get achievement %s: %v", key, err)
			continue
		}

		notification := &AchievementNotification{
			AchievementKey: key,
			Achievement:    achievement,
			Message:        n.FormatNotification(achievement),
		}
		notifications = append(notifications, notification)
	}

	return notifications, nil
}

func (n *AchievementNotifier) SendPreparedNotifications(ctx context.Context, userID int64, notifications []*AchievementNotification) error {
	for _, notification := range notifications {
		if err := n.sendNotification(ctx, userID, notification.Message); err != nil {
			log.Printf("[ACHIEVEMENT_NOTIFIER] Error sending prepared notification to user %d: %v", userID, err)
		}
	}
	return nil
}
