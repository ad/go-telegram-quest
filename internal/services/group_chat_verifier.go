package services

import (
	"context"
	"fmt"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type GroupChatVerifier struct {
	bot          *bot.Bot
	settingsRepo *db.SettingsRepository
}

func NewGroupChatVerifier(b *bot.Bot, settingsRepo *db.SettingsRepository) *GroupChatVerifier {
	return &GroupChatVerifier{
		bot:          b,
		settingsRepo: settingsRepo,
	}
}

func (v *GroupChatVerifier) IsVerificationEnabled() (bool, error) {
	chatID, err := v.settingsRepo.GetRequiredGroupChatID()
	if err != nil {
		return false, err
	}
	return chatID != 0, nil
}

func (v *GroupChatVerifier) VerifyMembership(ctx context.Context, userID int64) (bool, string, error) {
	chatID, err := v.settingsRepo.GetRequiredGroupChatID()
	if err != nil {
		return false, "", fmt.Errorf("failed to get group chat ID: %w", err)
	}

	inviteLink, err := v.settingsRepo.GetGroupChatInviteLink()
	if err != nil {
		return false, "", fmt.Errorf("failed to get invite link: %w", err)
	}

	member, err := v.bot.GetChatMember(ctx, &bot.GetChatMemberParams{
		ChatID: chatID,
		UserID: userID,
	})
	if err != nil {
		return false, inviteLink, fmt.Errorf("failed to get chat member: %w", err)
	}

	isMember := v.isValidMemberStatus(member.Type)
	return isMember, inviteLink, nil
}

func (v *GroupChatVerifier) isValidMemberStatus(status tgmodels.ChatMemberType) bool {
	switch status {
	case tgmodels.ChatMemberTypeOwner, tgmodels.ChatMemberTypeAdministrator, tgmodels.ChatMemberTypeMember:
		return true
	case tgmodels.ChatMemberTypeLeft, tgmodels.ChatMemberTypeBanned, tgmodels.ChatMemberTypeRestricted:
		return false
	default:
		return false
	}
}
