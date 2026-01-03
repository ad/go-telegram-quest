package db

import (
	"database/sql"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    first_name TEXT,
    last_name TEXT,
    username TEXT,
    is_blocked BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_order INTEGER UNIQUE NOT NULL,
    text TEXT NOT NULL,
    answer_type TEXT NOT NULL DEFAULT 'text',
    has_auto_check BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    is_deleted BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS step_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_id INTEGER NOT NULL REFERENCES steps(id),
    file_id TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS step_answers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    step_id INTEGER NOT NULL REFERENCES steps(id),
    answer TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_progress (
    user_id INTEGER NOT NULL REFERENCES users(id),
    step_id INTEGER NOT NULL REFERENCES steps(id),
    status TEXT NOT NULL DEFAULT 'pending',
    completed_at DATETIME,
    PRIMARY KEY (user_id, step_id)
);

CREATE TABLE IF NOT EXISTS user_answers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    step_id INTEGER NOT NULL REFERENCES steps(id),
    text_answer TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS answer_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    answer_id INTEGER NOT NULL REFERENCES user_answers(id),
    file_id TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_chat_state (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    last_task_message_id INTEGER,
    last_user_answer_message_id INTEGER,
    last_reaction_message_id INTEGER
);

CREATE TABLE IF NOT EXISTS admin_messages (
    key TEXT PRIMARY KEY,
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_state (
    user_id INTEGER PRIMARY KEY,
    current_state TEXT NOT NULL DEFAULT '',
    editing_step_id INTEGER DEFAULT 0,
    new_step_text TEXT DEFAULT '',
    new_step_type TEXT DEFAULT 'text',
    new_step_images TEXT DEFAULT '[]',
    new_step_answers TEXT DEFAULT '[]',
    editing_setting TEXT DEFAULT ''
);
`

const defaultSettings = `
INSERT OR IGNORE INTO settings (key, value) VALUES 
    ('welcome_message', 'Добро пожаловать в квест!'),
    ('final_message', 'Поздравляем! Вы прошли квест!'),
    ('correct_answer_message', '✅ Правильно!'),
    ('wrong_answer_message', '❌ Неверно, попробуйте ещё раз'),
    ('quest_state', 'not_started'),
    ('quest_not_started_message', 'Квест ещё не начался. Ожидайте объявления о старте!'),
    ('quest_paused_message', 'Квест временно приостановлен. Скоро мы продолжим!'),
    ('quest_completed_message', 'Квест завершён! Спасибо за участие!');
`

const migrations = `
ALTER TABLE users ADD COLUMN is_blocked BOOLEAN DEFAULT FALSE;
`

func InitSchema(db *sql.DB) error {
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	_, err = db.Exec(defaultSettings)
	if err != nil {
		return err
	}

	db.Exec(migrations)

	return nil
}
