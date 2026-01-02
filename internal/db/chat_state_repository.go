package db

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/models"
)

type ChatStateRepository struct {
	queue *DBQueue
}

func NewChatStateRepository(queue *DBQueue) *ChatStateRepository {
	return &ChatStateRepository{queue: queue}
}

func (r *ChatStateRepository) Save(state *models.ChatState) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, last_task_message_id, last_user_answer_message_id, last_reaction_message_id)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET
				last_task_message_id = excluded.last_task_message_id,
				last_user_answer_message_id = excluded.last_user_answer_message_id,
				last_reaction_message_id = excluded.last_reaction_message_id
		`, state.UserID, state.LastTaskMessageID, state.LastUserAnswerMessageID, state.LastReactionMessageID)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) Get(userID int64) (*models.ChatState, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT user_id, last_task_message_id, last_user_answer_message_id, last_reaction_message_id
			FROM user_chat_state WHERE user_id = ?
		`, userID)

		var state models.ChatState
		var taskMsgID, answerMsgID, reactionMsgID sql.NullInt64
		err := row.Scan(&state.UserID, &taskMsgID, &answerMsgID, &reactionMsgID)
		if err != nil {
			return nil, err
		}
		state.LastTaskMessageID = int(taskMsgID.Int64)
		state.LastUserAnswerMessageID = int(answerMsgID.Int64)
		state.LastReactionMessageID = int(reactionMsgID.Int64)
		return &state, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.ChatState), nil
}

func (r *ChatStateRepository) Clear(userID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM user_chat_state WHERE user_id = ?`, userID)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) UpdateTaskMessageID(userID int64, messageID int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, last_task_message_id)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET last_task_message_id = excluded.last_task_message_id
		`, userID, messageID)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) UpdateAnswerMessageID(userID int64, messageID int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, last_user_answer_message_id)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET last_user_answer_message_id = excluded.last_user_answer_message_id
		`, userID, messageID)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) UpdateReactionMessageID(userID int64, messageID int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, last_reaction_message_id)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET last_reaction_message_id = excluded.last_reaction_message_id
		`, userID, messageID)
		return nil, err
	})
	return err
}
