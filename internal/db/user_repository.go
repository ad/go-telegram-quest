package db

import (
	"database/sql"

	"github.com/ad/go-telegram-quest/internal/models"
)

type UserRepository struct {
	queue *DBQueue
}

func NewUserRepository(queue *DBQueue) *UserRepository {
	return &UserRepository{queue: queue}
}

func (r *UserRepository) CreateOrUpdate(user *models.User) error {
	_, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		_, err := db.Exec(`
			INSERT INTO users (id, first_name, last_name, username)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				first_name = excluded.first_name,
				last_name = excluded.last_name,
				username = excluded.username
		`, user.ID, user.FirstName, user.LastName, user.Username)
		return nil, err
	})
	return err
}

func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		row := db.QueryRow(`
			SELECT id, first_name, last_name, username, created_at
			FROM users WHERE id = ?
		`, id)

		var user models.User
		var firstName, lastName, username sql.NullString
		err := row.Scan(&user.ID, &firstName, &lastName, &username, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		user.FirstName = firstName.String
		user.LastName = lastName.String
		user.Username = username.String
		return &user, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.User), nil
}

func (r *UserRepository) GetAll() ([]*models.User, error) {
	result, err := r.queue.Execute(func(db *sql.DB) (interface{}, error) {
		rows, err := db.Query(`
			SELECT id, first_name, last_name, username, created_at
			FROM users ORDER BY created_at
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var users []*models.User
		for rows.Next() {
			var user models.User
			var firstName, lastName, username sql.NullString
			if err := rows.Scan(&user.ID, &firstName, &lastName, &username, &user.CreatedAt); err != nil {
				return nil, err
			}
			user.FirstName = firstName.String
			user.LastName = lastName.String
			user.Username = username.String
			users = append(users, &user)
		}
		return users, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*models.User), nil
}
