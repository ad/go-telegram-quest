package db

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/models"
)

type StickerPackRepository struct {
	queue *DBQueue
}

func NewStickerPackRepository(queue *DBQueue) *StickerPackRepository {
	return &StickerPackRepository{queue: queue}
}

func (r *StickerPackRepository) Create(userID int64, packName string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_sticker_packs (user_id, pack_name)
			VALUES (?, ?)
		`, userID, packName)
		return nil, err
	})
	return err
}

func (r *StickerPackRepository) GetByUserID(userID int64) (*models.UserStickerPack, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, user_id, pack_name, created_at
			FROM user_sticker_packs WHERE user_id = ?
		`, userID)

		var pack models.UserStickerPack
		err := row.Scan(&pack.ID, &pack.UserID, &pack.PackName, &pack.CreatedAt)
		if err != nil {
			return nil, err
		}
		return &pack, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.UserStickerPack), nil
}

func (r *StickerPackRepository) Exists(userID int64) (bool, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_sticker_packs WHERE user_id = ?
		`, userID).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}
