package db

import (
	"database/sql"
	"time"

	"github.com/ad/go-telegram-quest/internal/models"
)

type ProgressRepository struct {
	queue *DBQueue
}

func NewProgressRepository(queue *DBQueue) *ProgressRepository {
	return &ProgressRepository{queue: queue}
}

func (r *ProgressRepository) Create(progress *models.UserProgress) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO user_progress (user_id, step_id, status)
			VALUES (?, ?, ?)
		`, progress.UserID, progress.StepID, progress.Status)
		return nil, err
	})
	return err
}

func (r *ProgressRepository) Update(progress *models.UserProgress) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		if progress.Status == models.StatusApproved && progress.CompletedAt == nil {
			now := time.Now()
			progress.CompletedAt = &now
		}
		_, err := db.Exec(`
			UPDATE user_progress SET status = ?, completed_at = ?
			WHERE user_id = ? AND step_id = ?
		`, progress.Status, progress.CompletedAt, progress.UserID, progress.StepID)
		return nil, err
	})
	return err
}

func (r *ProgressRepository) GetByUserAndStep(userID, stepID int64) (*models.UserProgress, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT user_id, step_id, status, completed_at
			FROM user_progress WHERE user_id = ? AND step_id = ?
		`, userID, stepID)

		var progress models.UserProgress
		var completedAt sql.NullTime
		err := row.Scan(&progress.UserID, &progress.StepID, &progress.Status, &completedAt)
		if err != nil {
			return nil, err
		}
		if completedAt.Valid {
			progress.CompletedAt = &completedAt.Time
		}
		return &progress, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.UserProgress), nil
}

func (r *ProgressRepository) GetUserProgress(userID int64) ([]*models.UserProgress, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT user_id, step_id, status, completed_at
			FROM user_progress WHERE user_id = ?
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var progresses []*models.UserProgress
		for rows.Next() {
			var progress models.UserProgress
			var completedAt sql.NullTime
			if err := rows.Scan(&progress.UserID, &progress.StepID, &progress.Status, &completedAt); err != nil {
				return nil, err
			}
			if completedAt.Valid {
				progress.CompletedAt = &completedAt.Time
			}
			progresses = append(progresses, &progress)
		}
		return progresses, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.UserProgress), nil
}

func (r *ProgressRepository) CountByStep(stepID int64, status models.ProgressStatus) (int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_progress WHERE step_id = ? AND status = ?
		`, stepID, status).Scan(&count)
		return count, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}
