package db

import (
	"database/sql"
	"strings"

	"github.com/ad/go-telegram-quest/internal/models"
)

type StepRepository struct {
	queue *DBQueue
}

func NewStepRepository(queue *DBQueue) *StepRepository {
	return &StepRepository{queue: queue}
}

func (r *StepRepository) Create(step *models.Step) (int64, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		res, err := db.Exec(`
			INSERT INTO steps (step_order, text, answer_type, has_auto_check, is_active, is_deleted)
			VALUES (?, ?, ?, ?, ?, ?)
		`, step.StepOrder, step.Text, step.AnswerType, step.HasAutoCheck, step.IsActive, step.IsDeleted)
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

func (r *StepRepository) Update(step *models.Step) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			UPDATE steps SET
				step_order = ?,
				text = ?,
				answer_type = ?,
				has_auto_check = ?,
				is_active = ?,
				is_deleted = ?
			WHERE id = ?
		`, step.StepOrder, step.Text, step.AnswerType, step.HasAutoCheck, step.IsActive, step.IsDeleted, step.ID)
		return nil, err
	})
	return err
}

func (r *StepRepository) SoftDelete(id int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`UPDATE steps SET is_deleted = TRUE WHERE id = ?`, id)
		return nil, err
	})
	return err
}

func (r *StepRepository) GetActive() ([]*models.Step, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, step_order, text, answer_type, has_auto_check, is_active, is_deleted, created_at
			FROM steps
			WHERE is_active = TRUE AND is_deleted = FALSE
			ORDER BY step_order
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return r.scanSteps(db, rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.Step), nil
}

func (r *StepRepository) GetAll() ([]*models.Step, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, step_order, text, answer_type, has_auto_check, is_active, is_deleted, created_at
			FROM steps
			WHERE is_deleted = FALSE
			ORDER BY step_order
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return r.scanSteps(db, rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.Step), nil
}

func (r *StepRepository) HasCompletedProgress(stepID int64) (bool, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_progress WHERE step_id = ? AND status = 'approved'
		`, stepID).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (r *StepRepository) GetMaxOrder() (int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var maxOrder sql.NullInt64
		err := db.QueryRow(`SELECT MAX(step_order) FROM steps`).Scan(&maxOrder)
		if err != nil {
			return 0, err
		}
		if !maxOrder.Valid {
			return 0, nil
		}
		return int(maxOrder.Int64), nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (r *StepRepository) SetActive(id int64, active bool) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`UPDATE steps SET is_active = ? WHERE id = ?`, active, id)
		return nil, err
	})
	return err
}

func (r *StepRepository) UpdateText(id int64, text string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`UPDATE steps SET text = ? WHERE id = ?`, text, id)
		return nil, err
	})
	return err
}

func (r *StepRepository) DeleteImages(stepID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM step_images WHERE step_id = ?`, stepID)
		return nil, err
	})
	return err
}

func (r *StepRepository) DeleteAnswers(stepID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM step_answers WHERE step_id = ?`, stepID)
		return nil, err
	})
	return err
}

func (r *StepRepository) GetByID(id int64) (*models.Step, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, step_order, text, answer_type, has_auto_check, is_active, is_deleted, created_at
			FROM steps WHERE id = ?
		`, id)

		step, err := r.scanStep(row)
		if err != nil {
			return nil, err
		}
		return r.loadStepRelations(db, step)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Step), nil
}

func (r *StepRepository) GetNextActive(afterOrder int) (*models.Step, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, step_order, text, answer_type, has_auto_check, is_active, is_deleted, created_at
			FROM steps
			WHERE is_active = TRUE AND is_deleted = FALSE AND step_order > ?
			ORDER BY step_order
			LIMIT 1
		`, afterOrder)

		step, err := r.scanStep(row)
		if err != nil {
			return nil, err
		}
		return r.loadStepRelations(db, step)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Step), nil
}

func (r *StepRepository) AddImage(stepID int64, fileID string, position int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO step_images (step_id, file_id, position)
			VALUES (?, ?, ?)
		`, stepID, fileID, position)
		return nil, err
	})
	return err
}

func (r *StepRepository) ReplaceImage(stepID int64, oldPosition int, fileID string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			UPDATE step_images SET file_id = ? 
			WHERE step_id = ? AND position = ?
		`, fileID, stepID, oldPosition)
		return nil, err
	})
	return err
}

func (r *StepRepository) DeleteImage(stepID int64, position int) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		_, err = tx.Exec(`DELETE FROM step_images WHERE step_id = ? AND position = ?`, stepID, position)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`
			UPDATE step_images 
			SET position = position - 1 
			WHERE step_id = ? AND position > ?
		`, stepID, position)
		if err != nil {
			return nil, err
		}

		return nil, tx.Commit()
	})
	return err
}

func (r *StepRepository) GetImageCount(stepID int64) (int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM step_images WHERE step_id = ?`, stepID).Scan(&count)
		return count, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (r *StepRepository) AddAnswer(stepID int64, answer string) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO step_answers (step_id, answer)
			VALUES (?, ?)
		`, stepID, strings.ToLower(strings.TrimSpace(answer)))
		return nil, err
	})
	return err
}

func (r *StepRepository) scanStep(row *sql.Row) (*models.Step, error) {
	var step models.Step
	err := row.Scan(
		&step.ID, &step.StepOrder, &step.Text, &step.AnswerType,
		&step.HasAutoCheck, &step.IsActive, &step.IsDeleted, &step.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &step, nil
}

func (r *StepRepository) scanSteps(db *sql.DB, rows *sql.Rows) ([]*models.Step, error) {
	var steps []*models.Step
	for rows.Next() {
		var step models.Step
		if err := rows.Scan(
			&step.ID, &step.StepOrder, &step.Text, &step.AnswerType,
			&step.HasAutoCheck, &step.IsActive, &step.IsDeleted, &step.CreatedAt,
		); err != nil {
			return nil, err
		}
		loaded, err := r.loadStepRelations(db, &step)
		if err != nil {
			return nil, err
		}
		steps = append(steps, loaded)
	}
	return steps, rows.Err()
}

func (r *StepRepository) SwapStepOrder(stepID1, stepID2 int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		var order1, order2 int
		err = tx.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID1).Scan(&order1)
		if err != nil {
			return nil, err
		}

		err = tx.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID2).Scan(&order2)
		if err != nil {
			return nil, err
		}

		var maxOrder int
		err = tx.QueryRow(`SELECT COALESCE(MAX(step_order), 0) FROM steps`).Scan(&maxOrder)
		if err != nil {
			return nil, err
		}

		tempOrder := maxOrder + 1000
		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, tempOrder, stepID1)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, order1, stepID2)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, order2, stepID1)
		if err != nil {
			return nil, err
		}

		return nil, tx.Commit()
	})
	return err
}

func (r *StepRepository) MoveStepUp(stepID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		var currentOrder int
		err = tx.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID).Scan(&currentOrder)
		if err != nil {
			return nil, err
		}

		var prevStepID int64
		var prevOrder int
		err = tx.QueryRow(`
			SELECT id, step_order FROM steps 
			WHERE step_order < ? AND is_deleted = FALSE 
			ORDER BY step_order DESC LIMIT 1
		`, currentOrder).Scan(&prevStepID, &prevOrder)
		if err != nil {
			return nil, err
		}

		var maxOrder int
		err = tx.QueryRow(`SELECT COALESCE(MAX(step_order), 0) FROM steps`).Scan(&maxOrder)
		if err != nil {
			return nil, err
		}

		tempOrder := maxOrder + 1000
		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, tempOrder, stepID)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, currentOrder, prevStepID)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, prevOrder, stepID)
		if err != nil {
			return nil, err
		}

		return nil, tx.Commit()
	})
	return err
}

func (r *StepRepository) MoveStepDown(stepID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		var currentOrder int
		err = tx.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID).Scan(&currentOrder)
		if err != nil {
			return nil, err
		}

		var nextStepID int64
		var nextOrder int
		err = tx.QueryRow(`
			SELECT id, step_order FROM steps 
			WHERE step_order > ? AND is_deleted = FALSE 
			ORDER BY step_order ASC LIMIT 1
		`, currentOrder).Scan(&nextStepID, &nextOrder)
		if err != nil {
			return nil, err
		}

		var maxOrder int
		err = tx.QueryRow(`SELECT COALESCE(MAX(step_order), 0) FROM steps`).Scan(&maxOrder)
		if err != nil {
			return nil, err
		}

		tempOrder := maxOrder + 1000
		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, tempOrder, stepID)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, currentOrder, nextStepID)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`UPDATE steps SET step_order = ? WHERE id = ?`, nextOrder, stepID)
		if err != nil {
			return nil, err
		}

		return nil, tx.Commit()
	})
	return err
}

func (r *StepRepository) CanMoveUp(stepID int64) (bool, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var currentOrder int
		err := db.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID).Scan(&currentOrder)
		if err != nil {
			return false, err
		}

		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM steps 
			WHERE step_order < ? AND is_deleted = FALSE
		`, currentOrder).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (r *StepRepository) CanMoveDown(stepID int64) (bool, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var currentOrder int
		err := db.QueryRow(`SELECT step_order FROM steps WHERE id = ?`, stepID).Scan(&currentOrder)
		if err != nil {
			return false, err
		}

		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM steps 
			WHERE step_order > ? AND is_deleted = FALSE
		`, currentOrder).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (r *StepRepository) loadStepRelations(db *sql.DB, step *models.Step) (*models.Step, error) {
	imgRows, err := db.Query(`
		SELECT id, step_id, file_id, position
		FROM step_images WHERE step_id = ? ORDER BY position
	`, step.ID)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()

	for imgRows.Next() {
		var img models.StepImage
		if err := imgRows.Scan(&img.ID, &img.StepID, &img.FileID, &img.Position); err != nil {
			return nil, err
		}
		step.Images = append(step.Images, img)
	}

	ansRows, err := db.Query(`SELECT answer FROM step_answers WHERE step_id = ?`, step.ID)
	if err != nil {
		return nil, err
	}
	defer ansRows.Close()

	for ansRows.Next() {
		var answer string
		if err := ansRows.Scan(&answer); err != nil {
			return nil, err
		}
		step.Answers = append(step.Answers, answer)
	}

	return step, nil
}
