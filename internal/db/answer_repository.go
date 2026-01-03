package db

import (
	"database/sql"
	"strings"
	"time"
)

type AnswerRepository struct {
	queue *DBQueue
}

func NewAnswerRepository(queue *DBQueue) *AnswerRepository {
	return &AnswerRepository{queue: queue}
}

func (r *AnswerRepository) CreateTextAnswer(userID, stepID int64, textAnswer string) (int64, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		res, err := db.Exec(`
			INSERT INTO user_answers (user_id, step_id, text_answer)
			VALUES (?, ?, ?)
		`, userID, stepID, textAnswer)
		if err != nil {
			return nil, err
		}
		return res.LastInsertId()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (r *AnswerRepository) CreateImageAnswer(userID, stepID int64, fileIDs []string) (int64, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		res, err := db.Exec(`
			INSERT INTO user_answers (user_id, step_id)
			VALUES (?, ?)
		`, userID, stepID)
		if err != nil {
			return nil, err
		}

		answerID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}

		for i, fileID := range fileIDs {
			_, err = db.Exec(`
				INSERT INTO answer_images (answer_id, file_id, position)
				VALUES (?, ?, ?)
			`, answerID, fileID, i)
			if err != nil {
				return nil, err
			}
		}

		return answerID, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (r *AnswerRepository) GetStepAnswers(stepID int64) ([]string, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`SELECT answer FROM step_answers WHERE step_id = ?`, stepID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var answers []string
		for rows.Next() {
			var answer string
			if err := rows.Scan(&answer); err != nil {
				return nil, err
			}
			answers = append(answers, answer)
		}
		return answers, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

func (r *AnswerRepository) AddStepAnswer(stepID int64, answer string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO step_answers (step_id, answer)
			VALUES (?, ?)
		`, stepID, strings.ToLower(strings.TrimSpace(answer)))
		return nil, err
	})
	return err
}

func (r *AnswerRepository) DeleteStepAnswer(stepID int64, answer string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			DELETE FROM step_answers WHERE step_id = ? AND answer = ?
		`, stepID, strings.ToLower(strings.TrimSpace(answer)))
		return nil, err
	})
	return err
}

func (r *AnswerRepository) GetUserAnswerTimes(userID int64) ([]time.Time, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT created_at FROM user_answers 
			WHERE user_id = ? 
			ORDER BY created_at ASC
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var times []time.Time
		for rows.Next() {
			var t time.Time
			if err := rows.Scan(&t); err != nil {
				return nil, err
			}
			times = append(times, t)
		}
		return times, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]time.Time), nil
}

func (r *AnswerRepository) CountUserAnswers(userID int64) (int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_answers WHERE user_id = ?
		`, userID).Scan(&count)
		return count, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (r *AnswerRepository) CountUserAnswersByStep(userID int64) (map[int64]int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT step_id, COUNT(*) as attempts 
			FROM user_answers 
			WHERE user_id = ? 
			GROUP BY step_id
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		counts := make(map[int64]int)
		for rows.Next() {
			var stepID int64
			var count int
			if err := rows.Scan(&stepID, &count); err != nil {
				return nil, err
			}
			counts[stepID] = count
		}
		return counts, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[int64]int), nil
}
