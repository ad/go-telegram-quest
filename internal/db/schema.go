package db

import (
	"database/sql"
	"log"
	"strings"
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
    correct_answer_image TEXT,
    hint_text TEXT DEFAULT '',
    hint_image TEXT DEFAULT '',
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
    hint_used BOOLEAN DEFAULT FALSE,
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
    last_reaction_message_id INTEGER,
    hint_message_id INTEGER DEFAULT 0,
    current_step_hint_used BOOLEAN DEFAULT FALSE
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
    editing_setting TEXT DEFAULT '',
    new_hint_text TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS achievements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    category TEXT NOT NULL,
    type TEXT NOT NULL,
    is_unique BOOLEAN DEFAULT FALSE,
    conditions TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS user_achievements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    achievement_id INTEGER NOT NULL REFERENCES achievements(id),
    earned_at DATETIME NOT NULL,
    is_retroactive BOOLEAN DEFAULT FALSE,
    UNIQUE(user_id, achievement_id)
);

CREATE INDEX IF NOT EXISTS idx_user_achievements_user_id ON user_achievements(user_id);
CREATE INDEX IF NOT EXISTS idx_user_achievements_achievement_id ON user_achievements(achievement_id);
CREATE INDEX IF NOT EXISTS idx_user_achievements_earned_at ON user_achievements(earned_at);
CREATE INDEX IF NOT EXISTS idx_achievements_key ON achievements(key);
CREATE INDEX IF NOT EXISTS idx_achievements_category ON achievements(category);
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
ALTER TABLE steps ADD COLUMN correct_answer_image TEXT;
ALTER TABLE steps ADD COLUMN hint_text TEXT DEFAULT '';
ALTER TABLE steps ADD COLUMN hint_image TEXT DEFAULT '';
ALTER TABLE user_chat_state ADD COLUMN hint_message_id INTEGER DEFAULT 0;
ALTER TABLE user_chat_state ADD COLUMN current_step_hint_used BOOLEAN DEFAULT FALSE;
ALTER TABLE user_answers ADD COLUMN hint_used BOOLEAN DEFAULT FALSE;
ALTER TABLE admin_state ADD COLUMN new_hint_text TEXT DEFAULT '';
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

	// Split migrations and execute them one by one
	migrationStatements := strings.Split(migrations, ";")
	for i, stmt := range migrationStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Ignore errors for migrations as columns might already exist, but log them for debugging
		if _, err := db.Exec(stmt); err != nil {
			// log.Printf("Migration %d failed: %s. Error: %v", i, stmt, err)
		} else {
			log.Printf("Migration %d executed: %s", i, stmt)
		}
	}

	// Initialize default achievements
	if err := InitializeDefaultAchievements(db); err != nil {
		log.Printf("Failed to initialize default achievements: %v", err)
		return err
	}

	return nil
}
