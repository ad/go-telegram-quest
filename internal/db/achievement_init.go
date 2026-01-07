package db

import (
	"database/sql"
	"log"

	"github.com/ad/go-telegram-quest/internal/models"
)

func InitializeDefaultAchievements(db *sql.DB) error {
	achievements := getDefaultAchievements()

	for _, achievement := range achievements {
		// Check if achievement already exists
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM achievements WHERE key = ?", achievement.Key).Scan(&count)
		if err != nil {
			return err
		}

		if count == 0 {
			conditionsJSON, err := achievement.Conditions.ToJSON()
			if err != nil {
				return err
			}

			_, err = db.Exec(`
				INSERT INTO achievements (key, name, description, category, type, is_unique, conditions, is_active)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, achievement.Key, achievement.Name, achievement.Description, achievement.Category,
				achievement.Type, achievement.IsUnique, conditionsJSON, achievement.IsActive)
			if err != nil {
				log.Printf("Failed to create achievement %s: %v", achievement.Key, err)
				return err
			}
			// log.Printf("Created achievement: %s", achievement.Key)
		} else {
			// log.Printf("Achievement %s already exists, skipping", achievement.Key)
		}
	}

	return nil
}

func getDefaultAchievements() []*models.Achievement {
	var achievements []*models.Achievement

	// Unique achievements
	achievements = append(achievements, &models.Achievement{
		Key:         "pioneer",
		Name:        "Пионер",
		Description: "Первый участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(1),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "second_place",
		Name:        "Второй",
		Description: "Второй участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(2),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "third_place",
		Name:        "Третий",
		Description: "Третий участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(3),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "fourth_place",
		Name:        "Четвёртый",
		Description: "Четвёртый участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(4),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "fifth_place",
		Name:        "Пятый",
		Description: "Пятый участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(5),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "sixth_place",
		Name:        "Шестой",
		Description: "Шестой участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(6),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "seventh_place",
		Name:        "Седьмой",
		Description: "Седьмой участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(7),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "eighth_place",
		Name:        "Восьмой",
		Description: "Восьмой участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(8),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "ninth_place",
		Name:        "Девятый",
		Description: "Девятый участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(9),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "tenth_place",
		Name:        "Десятый",
		Description: "Десятый участник квеста",
		Category:    models.CategoryUnique,
		Type:        models.TypeUnique,
		IsUnique:    true,
		Conditions: models.AchievementConditions{
			Position: intPtr(10),
		},
		IsActive: true,
	})

	// Progress-based achievements
	achievements = append(achievements, &models.Achievement{
		Key:         "beginner_5",
		Name:        "Начинающий",
		Description: "Дать 5 правильных ответов",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: intPtr(5),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "experienced_10",
		Name:        "Опытный",
		Description: "Дать 10 правильных ответов",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: intPtr(10),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "advanced_15",
		Name:        "Продвинутый",
		Description: "Дать 15 правильных ответов",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: intPtr(15),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "expert_20",
		Name:        "Эксперт",
		Description: "Дать 20 правильных ответов",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: intPtr(20),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "master_25",
		Name:        "Мастер",
		Description: "Дать 25 правильных ответов",
		Category:    models.CategoryProgress,
		Type:        models.TypeProgressBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CorrectAnswers: intPtr(25),
		},
		IsActive: true,
	})

	// Completion-based achievements (CorrectAnswers определяется динамически из количества активных шагов)
	achievements = append(achievements, &models.Achievement{
		Key:         "winner",
		Name:        "Победитель",
		Description: "Завершить весь квест",
		Category:    models.CategoryCompletion,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions:  models.AchievementConditions{},
		IsActive:    true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "perfect_path",
		Name:        "Идеальный Путь",
		Description: "Завершить квест без единой ошибки",
		Category:    models.CategoryCompletion,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			NoErrors: boolPtr(true),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "self_sufficient",
		Name:        "Самодостаточный",
		Description: "Завершить квест без использования подсказок",
		Category:    models.CategoryCompletion,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			NoHints: boolPtr(true),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "lightning",
		Name:        "Молния",
		Description: "Завершить квест менее чем за 10 минут",
		Category:    models.CategoryCompletion,
		Type:        models.TypeTimeBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CompletionTimeMinutes: intPtr(10),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "rocket",
		Name:        "Ракета",
		Description: "Завершить квест менее чем за 60 минут",
		Category:    models.CategoryCompletion,
		Type:        models.TypeTimeBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CompletionTimeMinutes: intPtr(60),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "cheater",
		Name:        "Жулик",
		Description: "Завершить квест менее чем за 5 минут",
		Category:    models.CategoryCompletion,
		Type:        models.TypeTimeBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			CompletionTimeMinutes: intPtr(5),
		},
		IsActive: true,
	})

	// Hint-based achievements
	achievements = append(achievements, &models.Achievement{
		Key:         "hint_5",
		Name:        "Подсказочный 5",
		Description: "Использовать 5 подсказок",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintCount: intPtr(5),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "hint_10",
		Name:        "Подсказочный 10",
		Description: "Использовать 10 подсказок",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintCount: intPtr(10),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "hint_15",
		Name:        "Подсказочный 15",
		Description: "Использовать 15 подсказок",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintCount: intPtr(15),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "hint_25",
		Name:        "Подсказочный 25",
		Description: "Использовать 25 подсказок",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintCount: intPtr(25),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "hint_30",
		Name:        "Подсказочный 30",
		Description: "Использовать 30 подсказок",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintCount: intPtr(30),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "hint_master",
		Name:        "Мастер Подсказок",
		Description: "Использовать все доступные подсказки в квесте",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			AllHintsUsed: boolPtr(true),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "skeptic",
		Name:        "Скептик",
		Description: "Использовать подсказку на первом задании",
		Category:    models.CategoryHints,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			HintOnFirstTask: boolPtr(true),
		},
		IsActive: true,
	})

	// Special achievements
	achievements = append(achievements, &models.Achievement{
		Key:         "photographer",
		Name:        "Фотограф",
		Description: "Отправить фото для фото-задания",
		Category:    models.CategorySpecial,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			PhotoSubmitted: boolPtr(true),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "bullseye",
		Name:        "В точку",
		Description: "Ответить правильно на 10 заданий подряд с первой попытки",
		Category:    models.CategorySpecial,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			ConsecutiveCorrect: intPtr(10),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "secret_agent",
		Name:        "Секретный Агент",
		Description: "Ответить 'сезам откройся' на любое задание",
		Category:    models.CategorySpecial,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			SpecificAnswer: stringPtr("сезам откройся"),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "curious",
		Name:        "Любопытный",
		Description: "Зайти в квест, но не отвечать на вопросы 24 часа",
		Category:    models.CategorySpecial,
		Type:        models.TypeTimeBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			InactiveHours: intPtr(24),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "paparazzi",
		Name:        "Папарацци",
		Description: "Отправить фото для текстового задания",
		Category:    models.CategorySpecial,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			PhotoOnTextTask: boolPtr(true),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "fan",
		Name:        "Фанат",
		Description: "Отправить сообщение после завершения всего квеста",
		Category:    models.CategorySpecial,
		Type:        models.TypeActionBased,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			PostCompletion: boolPtr(true),
		},
		IsActive: true,
	})

	// Composite achievements
	achievements = append(achievements, &models.Achievement{
		Key:         "super_collector",
		Name:        "Суперколлекционер",
		Description: "Собрал все достижения за прогресс (5-25 правильных ответов)",
		Category:    models.CategoryComposite,
		Type:        models.TypeComposite,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			RequiredAchievements: []string{"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25"},
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "super_brain",
		Name:        "Супермозг",
		Description: "Завершить квест без ошибок, подсказок и менее чем за 30 минут",
		Category:    models.CategoryComposite,
		Type:        models.TypeComposite,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			NoErrors:              boolPtr(true),
			NoHints:               boolPtr(true),
			CompletionTimeMinutes: intPtr(30),
		},
		IsActive: true,
	})

	achievements = append(achievements, &models.Achievement{
		Key:         "legend",
		Name:        "Легенда",
		Description: "Собрать достижения первых 10 участников, все прогрессивные и все за завершение",
		Category:    models.CategoryComposite,
		Type:        models.TypeComposite,
		IsUnique:    false,
		Conditions: models.AchievementConditions{
			RequiredAchievements: []string{
				"pioneer", "second_place", "third_place", "fourth_place", "fifth_place",
				"sixth_place", "seventh_place", "eighth_place", "ninth_place", "tenth_place",
				"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25",
				"winner", "perfect_path", "self_sufficient", "lightning", "rocket", "cheater",
			},
		},
		IsActive: true,
	})

	return achievements
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
