package db

import (
	"database/sql"
)

type AdminMessage struct {
	Key       string
	ChatID    int64
	MessageID int
}

type AdminMessagesRepository struct {
	queue *DBQueue
}

func NewAdminMessagesRepository(queue *DBQueue) *AdminMessagesRepository {
	return &AdminMessagesRepository{queue: queue}
}

func (r *AdminMessagesRepository) Get(key string) (*AdminMessage, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`SELECT key, chat_id, message_id FROM admin_messages WHERE key = ?`, key)
		var msg AdminMessage
		err := row.Scan(&msg.Key, &msg.ChatID, &msg.MessageID)
		if err != nil {
			return nil, err
		}
		return &msg, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*AdminMessage), nil
}

func (r *AdminMessagesRepository) Set(key string, chatID int64, messageID int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO admin_messages (key, chat_id, message_id) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET chat_id = excluded.chat_id, message_id = excluded.message_id
		`, key, chatID, messageID)
		return nil, err
	})
	return err
}

func (r *AdminMessagesRepository) Delete(key string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM admin_messages WHERE key = ?`, key)
		return nil, err
	})
	return err
}
