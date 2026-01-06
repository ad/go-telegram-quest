package models

import "time"

type UserStickerPack struct {
	ID        int64
	UserID    int64
	PackName  string
	CreatedAt time.Time
}
