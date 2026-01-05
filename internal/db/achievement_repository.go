package db

import (
	"database/sql"
	"time"

	"github.com/ad/go-telegram-quest/internal/models"
)

type AchievementRepository struct {
	queue *DBQueue
}

func NewAchievementRepository(queue *DBQueue) *AchievementRepository {
	return &AchievementRepository{queue: queue}
}

func (r *AchievementRepository) Create(achievement *models.Achievement) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		conditionsJSON, err := achievement.Conditions.ToJSON()
		if err != nil {
			return nil, err
		}

		res, err := db.Exec(`
			INSERT INTO achievements (key, name, description, category, type, is_unique, conditions, is_active)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, achievement.Key, achievement.Name, achievement.Description, achievement.Category,
			achievement.Type, achievement.IsUnique, conditionsJSON, achievement.IsActive)
		if err != nil {
			return nil, err
		}

		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		achievement.ID = id
		return nil, nil
	})
	return err
}

func (r *AchievementRepository) GetByID(id int64) (*models.Achievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, key, name, description, category, type, is_unique, conditions, created_at, is_active
			FROM achievements WHERE id = ?
		`, id)
		return scanAchievement(row)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Achievement), nil
}

func (r *AchievementRepository) GetByKey(key string) (*models.Achievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, key, name, description, category, type, is_unique, conditions, created_at, is_active
			FROM achievements WHERE key = ?
		`, key)
		return scanAchievement(row)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.Achievement), nil
}

func (r *AchievementRepository) GetAll() ([]*models.Achievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, key, name, description, category, type, is_unique, conditions, created_at, is_active
			FROM achievements ORDER BY created_at
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.Achievement), nil
}

func (r *AchievementRepository) GetActive() ([]*models.Achievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, key, name, description, category, type, is_unique, conditions, created_at, is_active
			FROM achievements WHERE is_active = 1 ORDER BY created_at
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.Achievement), nil
}

func (r *AchievementRepository) GetByCategory(category models.AchievementCategory) ([]*models.Achievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, key, name, description, category, type, is_unique, conditions, created_at, is_active
			FROM achievements WHERE category = ? AND is_active = 1 ORDER BY created_at
		`, category)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.Achievement), nil
}

func (r *AchievementRepository) Update(achievement *models.Achievement) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		conditionsJSON, err := achievement.Conditions.ToJSON()
		if err != nil {
			return nil, err
		}

		_, err = db.Exec(`
			UPDATE achievements SET 
				name = ?, description = ?, category = ?, type = ?, 
				is_unique = ?, conditions = ?, is_active = ?
			WHERE id = ?
		`, achievement.Name, achievement.Description, achievement.Category,
			achievement.Type, achievement.IsUnique, conditionsJSON, achievement.IsActive, achievement.ID)
		return nil, err
	})
	return err
}

func (r *AchievementRepository) AssignToUser(userID, achievementID int64, earnedAt time.Time, isRetroactive bool) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT OR IGNORE INTO user_achievements (user_id, achievement_id, earned_at, is_retroactive)
			VALUES (?, ?, ?, ?)
		`, userID, achievementID, earnedAt, isRetroactive)
		return nil, err
	})
	return err
}

func (r *AchievementRepository) GetUserAchievements(userID int64) ([]*models.UserAchievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, user_id, achievement_id, earned_at, is_retroactive
			FROM user_achievements WHERE user_id = ? ORDER BY earned_at
		`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanUserAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.UserAchievement), nil
}

func (r *AchievementRepository) GetUserAchievementsByCategory(userID int64, category models.AchievementCategory) ([]*models.UserAchievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT ua.id, ua.user_id, ua.achievement_id, ua.earned_at, ua.is_retroactive
			FROM user_achievements ua
			JOIN achievements a ON ua.achievement_id = a.id
			WHERE ua.user_id = ? AND a.category = ?
			ORDER BY ua.earned_at
		`, userID, category)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanUserAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.UserAchievement), nil
}

func (r *AchievementRepository) HasUserAchievement(userID int64, achievementKey string) (bool, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_achievements ua
			JOIN achievements a ON ua.achievement_id = a.id
			WHERE ua.user_id = ? AND a.key = ?
		`, userID, achievementKey).Scan(&count)
		return count > 0, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (r *AchievementRepository) GetAchievementHolders(achievementKey string) ([]int64, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT ua.user_id FROM user_achievements ua
			JOIN achievements a ON ua.achievement_id = a.id
			WHERE a.key = ?
			ORDER BY ua.earned_at
		`, achievementKey)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var userIDs []int64
		for rows.Next() {
			var userID int64
			if err := rows.Scan(&userID); err != nil {
				return nil, err
			}
			userIDs = append(userIDs, userID)
		}
		return userIDs, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]int64), nil
}

func (r *AchievementRepository) GetAchievementStats() (map[string]int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT a.key, COUNT(ua.id) as user_count
			FROM achievements a
			LEFT JOIN user_achievements ua ON a.id = ua.achievement_id
			GROUP BY a.id, a.key
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		stats := make(map[string]int)
		for rows.Next() {
			var key string
			var count int
			if err := rows.Scan(&key, &count); err != nil {
				return nil, err
			}
			stats[key] = count
		}
		return stats, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]int), nil
}

func (r *AchievementRepository) CountUserAchievements(userID int64) (int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM user_achievements WHERE user_id = ?
		`, userID).Scan(&count)
		return count, err
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (r *AchievementRepository) GetAllUserAchievements() ([]*models.UserAchievement, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, user_id, achievement_id, earned_at, is_retroactive
			FROM user_achievements ORDER BY earned_at
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanUserAchievements(rows)
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.UserAchievement), nil
}

func (r *AchievementRepository) GetUsersWithAchievementCount() (map[int64]int, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT user_id, COUNT(*) as achievement_count
			FROM user_achievements
			GROUP BY user_id
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		counts := make(map[int64]int)
		for rows.Next() {
			var userID int64
			var count int
			if err := rows.Scan(&userID, &count); err != nil {
				return nil, err
			}
			counts[userID] = count
		}
		return counts, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[int64]int), nil
}

func (r *AchievementRepository) DeleteUserAchievements(userID int64) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`DELETE FROM user_achievements WHERE user_id = ?`, userID)
		return nil, err
	})
	return err
}

func scanAchievement(row *sql.Row) (*models.Achievement, error) {
	var achievement models.Achievement
	var conditionsJSON string
	err := row.Scan(
		&achievement.ID, &achievement.Key, &achievement.Name, &achievement.Description,
		&achievement.Category, &achievement.Type, &achievement.IsUnique,
		&conditionsJSON, &achievement.CreatedAt, &achievement.IsActive,
	)
	if err != nil {
		return nil, err
	}

	conditions, err := models.ParseAchievementConditions(conditionsJSON)
	if err != nil {
		return nil, err
	}
	achievement.Conditions = *conditions
	return &achievement, nil
}

func scanAchievements(rows *sql.Rows) ([]*models.Achievement, error) {
	var achievements []*models.Achievement
	for rows.Next() {
		var achievement models.Achievement
		var conditionsJSON string
		if err := rows.Scan(
			&achievement.ID, &achievement.Key, &achievement.Name, &achievement.Description,
			&achievement.Category, &achievement.Type, &achievement.IsUnique,
			&conditionsJSON, &achievement.CreatedAt, &achievement.IsActive,
		); err != nil {
			return nil, err
		}

		conditions, err := models.ParseAchievementConditions(conditionsJSON)
		if err != nil {
			return nil, err
		}
		achievement.Conditions = *conditions
		achievements = append(achievements, &achievement)
	}
	return achievements, rows.Err()
}

func scanUserAchievements(rows *sql.Rows) ([]*models.UserAchievement, error) {
	var userAchievements []*models.UserAchievement
	for rows.Next() {
		var ua models.UserAchievement
		if err := rows.Scan(&ua.ID, &ua.UserID, &ua.AchievementID, &ua.EarnedAt, &ua.IsRetroactive); err != nil {
			return nil, err
		}
		userAchievements = append(userAchievements, &ua)
	}
	return userAchievements, rows.Err()
}
