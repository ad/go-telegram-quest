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
			INSERT INTO user_chat_state (user_id, last_task_message_id, last_user_answer_message_id, last_reaction_message_id, hint_message_id, current_step_hint_used)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET
				last_task_message_id = excluded.last_task_message_id,
				last_user_answer_message_id = excluded.last_user_answer_message_id,
				last_reaction_message_id = excluded.last_reaction_message_id,
				hint_message_id = excluded.hint_message_id,
				current_step_hint_used = excluded.current_step_hint_used
		`, state.UserID, state.LastTaskMessageID, state.LastUserAnswerMessageID, state.LastReactionMessageID, state.HintMessageID, state.CurrentStepHintUsed)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) Get(userID int64) (*models.ChatState, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT user_id, last_task_message_id, last_user_answer_message_id, last_reaction_message_id, hint_message_id, current_step_hint_used
			FROM user_chat_state WHERE user_id = ?
		`, userID)

		var state models.ChatState
		var taskMsgID, answerMsgID, reactionMsgID, hintMsgID sql.NullInt64
		var hintUsed sql.NullBool
		err := row.Scan(&state.UserID, &taskMsgID, &answerMsgID, &reactionMsgID, &hintMsgID, &hintUsed)
		if err != nil {
			return nil, err
		}
		state.LastTaskMessageID = int(taskMsgID.Int64)
		state.LastUserAnswerMessageID = int(answerMsgID.Int64)
		state.LastReactionMessageID = int(reactionMsgID.Int64)
		state.HintMessageID = int(hintMsgID.Int64)
		state.CurrentStepHintUsed = hintUsed.Bool
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

func (r *ChatStateRepository) UpdateHintMessageID(userID int64, messageID int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, hint_message_id)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET hint_message_id = excluded.hint_message_id
		`, userID, messageID)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) SetHintUsed(userID int64, used bool) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, current_step_hint_used)
			VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET current_step_hint_used = excluded.current_step_hint_used
		`, userID, used)
		return nil, err
	})
	return err
}

func (r *ChatStateRepository) ResetHintUsed(userID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_chat_state (user_id, current_step_hint_used, hint_message_id)
			VALUES (?, FALSE, 0)
			ON CONFLICT(user_id) DO UPDATE SET 
				current_step_hint_used = FALSE,
				hint_message_id = 0
		`, userID)
		return nil, err
	})
	return err
}
