package db

import (
	"database/sql"
	"testing"

	"github.com/ad/go-telegram-quest/internal/models"
	_ "modernc.org/sqlite"
)

func TestInitializeDefaultAchievements(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	repo := NewAchievementRepository(NewDBQueue(db))

	// Test that all expected achievements are created
	expectedAchievements := map[string]struct {
		name        string
		category    models.AchievementCategory
		isUnique    bool
		hasPosition bool
	}{
		"pioneer":         {"Пионер", models.CategoryUnique, true, true},
		"second_place":    {"Второй", models.CategoryUnique, true, true},
		"third_place":     {"Третий", models.CategoryUnique, true, true},
		"fourth_place":    {"Четвёртый", models.CategoryUnique, true, true},
		"fifth_place":     {"Пятый", models.CategoryUnique, true, true},
		"sixth_place":     {"Шестой", models.CategoryUnique, true, true},
		"seventh_place":   {"Седьмой", models.CategoryUnique, true, true},
		"eighth_place":    {"Восьмой", models.CategoryUnique, true, true},
		"ninth_place":     {"Девятый", models.CategoryUnique, true, true},
		"tenth_place":     {"Десятый", models.CategoryUnique, true, true},
		"winner_1":        {"1-й победитель", models.CategoryUnique, true, false},
		"winner_2":        {"2-й победитель", models.CategoryUnique, true, false},
		"winner_3":        {"3-й победитель", models.CategoryUnique, true, false},
		"beginner_5":      {"Начинающий", models.CategoryProgress, false, false},
		"experienced_10":  {"Опытный", models.CategoryProgress, false, false},
		"advanced_15":     {"Продвинутый", models.CategoryProgress, false, false},
		"expert_20":       {"Эксперт", models.CategoryProgress, false, false},
		"master_25":       {"Мастер", models.CategoryProgress, false, false},
		"winner":          {"Победитель", models.CategoryCompletion, false, false},
		"perfect_path":    {"Идеальный Путь", models.CategoryCompletion, false, false},
		"self_sufficient": {"Самодостаточный", models.CategoryCompletion, false, false},
		"lightning":       {"Молния", models.CategoryCompletion, false, false},
		"rocket":          {"Ракета", models.CategoryCompletion, false, false},
		"cheater":         {"Жулик", models.CategoryCompletion, false, false},
		"hint_5":          {"Подсказочный 5", models.CategoryHints, false, false},
		"hint_10":         {"Подсказочный 10", models.CategoryHints, false, false},
		"hint_15":         {"Подсказочный 15", models.CategoryHints, false, false},
		"hint_25":         {"Подсказочный 25", models.CategoryHints, false, false},
		"hint_30":         {"Подсказочный 30", models.CategoryHints, false, false},
		"hint_master":     {"Мастер Подсказок", models.CategoryHints, false, false},
		"skeptic":         {"Скептик", models.CategoryHints, false, false},
		"photographer":    {"Фотограф", models.CategorySpecial, false, false},
		"bullseye":        {"В точку", models.CategorySpecial, false, false},
		"secret_agent":    {"Секретный Агент", models.CategorySpecial, false, false},
		"curious":         {"Любопытный", models.CategorySpecial, false, false},
		"paparazzi":       {"Папарацци", models.CategorySpecial, false, false},
		"fan":             {"Фанат", models.CategorySpecial, false, false},
		"restart":         {"Начать с начала", models.CategorySpecial, false, false},
		"writer":          {"Писатель", models.CategorySpecial, false, false},
		"veteran":         {"Ветеран игр", models.CategorySpecial, false, false},
		"activity":        {"За активность", models.CategorySpecial, false, false},
		"wow":             {"Вау! За отличный ответ", models.CategorySpecial, false, false},
		"super_collector": {"Суперколлекционер", models.CategoryComposite, false, false},
		"super_brain":     {"Супермозг", models.CategoryComposite, false, false},
		"legend":          {"Легенда", models.CategoryComposite, false, false},
		"asterisk":        {"Вопрос со звёздочкой", models.CategorySpecial, false, false},
		"unseen":          {"Невидимый собеседник", models.CategorySpecial, false, false},
		"voice":           {"Голос свыше", models.CategorySpecial, false, false},
	}

	for key, expected := range expectedAchievements {
		achievement, err := repo.GetByKey(key)
		if err != nil {
			t.Errorf("Failed to get achievement %s: %v", key, err)
			continue
		}

		if achievement.Name != expected.name {
			t.Errorf("Achievement %s: expected name %s, got %s", key, expected.name, achievement.Name)
		}

		if achievement.Category != expected.category {
			t.Errorf("Achievement %s: expected category %s, got %s", key, expected.category, achievement.Category)
		}

		if achievement.IsUnique != expected.isUnique {
			t.Errorf("Achievement %s: expected IsUnique %v, got %v", key, expected.isUnique, achievement.IsUnique)
		}

		if !achievement.IsActive {
			t.Errorf("Achievement %s: expected to be active", key)
		}

		if expected.hasPosition && achievement.Conditions.Position == nil {
			t.Errorf("Achievement %s: expected to have position condition", key)
		}
	}

	// Test total count
	allAchievements, err := repo.GetAll()
	if err != nil {
		t.Fatalf("Failed to get all achievements: %v", err)
	}

	if len(allAchievements) != len(expectedAchievements) {
		t.Errorf("Expected %d achievements, got %d", len(expectedAchievements), len(allAchievements))
	}
}

func TestAchievementConditionParsing(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	repo := NewAchievementRepository(NewDBQueue(db))

	testCases := []struct {
		key                string
		expectedConditions func(*models.AchievementConditions) bool
	}{
		{
			key: "beginner_5",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.CorrectAnswers != nil && *c.CorrectAnswers == 5
			},
		},
		{
			key: "pioneer",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.Position != nil && *c.Position == 1
			},
		},
		{
			key: "perfect_path",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.NoErrors != nil && *c.NoErrors == true
			},
		},
		{
			key: "lightning",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.CompletionTimeMinutes != nil && *c.CompletionTimeMinutes == 10
			},
		},
		{
			key: "hint_5",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.HintCount != nil && *c.HintCount == 5
			},
		},
		{
			key: "secret_agent",
			expectedConditions: func(c *models.AchievementConditions) bool {
				return c.SpecificAnswer != nil && *c.SpecificAnswer == "сезам откройся"
			},
		},
		{
			key: "super_collector",
			expectedConditions: func(c *models.AchievementConditions) bool {
				expected := []string{"beginner_5", "experienced_10", "advanced_15", "expert_20", "master_25"}
				if len(c.RequiredAchievements) != len(expected) {
					return false
				}
				for i, req := range expected {
					if c.RequiredAchievements[i] != req {
						return false
					}
				}
				return true
			},
		},
	}

	for _, tc := range testCases {
		achievement, err := repo.GetByKey(tc.key)
		if err != nil {
			t.Errorf("Failed to get achievement %s: %v", tc.key, err)
			continue
		}

		if !tc.expectedConditions(&achievement.Conditions) {
			t.Errorf("Achievement %s: conditions validation failed", tc.key)
		}
	}
}

func TestInitializeDefaultAchievementsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	repo := NewAchievementRepository(NewDBQueue(db))

	// Get initial count
	initialAchievements, err := repo.GetAll()
	if err != nil {
		t.Fatalf("Failed to get initial achievements: %v", err)
	}
	initialCount := len(initialAchievements)

	// Run initialization again
	if err := InitializeDefaultAchievements(db); err != nil {
		t.Fatalf("Failed to initialize achievements second time: %v", err)
	}

	// Check that count hasn't changed
	finalAchievements, err := repo.GetAll()
	if err != nil {
		t.Fatalf("Failed to get final achievements: %v", err)
	}
	finalCount := len(finalAchievements)

	if finalCount != initialCount {
		t.Errorf("Expected achievement count to remain %d after second initialization, got %d", initialCount, finalCount)
	}
}

func TestAchievementConditionJSONSerialization(t *testing.T) {
	testCases := []struct {
		name       string
		conditions models.AchievementConditions
	}{
		{
			name: "simple_correct_answers",
			conditions: models.AchievementConditions{
				CorrectAnswers: intPtr(5),
			},
		},
		{
			name: "complex_conditions",
			conditions: models.AchievementConditions{
				CorrectAnswers:        intPtr(25),
				CompletionTimeMinutes: intPtr(10),
				NoErrors:              boolPtr(true),
				NoHints:               boolPtr(true),
			},
		},
		{
			name: "composite_achievement",
			conditions: models.AchievementConditions{
				RequiredAchievements: []string{"beginner_5", "expert_20"},
			},
		},
		{
			name: "special_conditions",
			conditions: models.AchievementConditions{
				SpecificAnswer:     stringPtr("test answer"),
				PhotoSubmitted:     boolPtr(true),
				ConsecutiveCorrect: intPtr(10),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test serialization
			jsonStr, err := tc.conditions.ToJSON()
			if err != nil {
				t.Fatalf("Failed to serialize conditions: %v", err)
			}

			// Test deserialization
			parsed, err := models.ParseAchievementConditions(jsonStr)
			if err != nil {
				t.Fatalf("Failed to parse conditions: %v", err)
			}

			// Compare key fields
			if tc.conditions.CorrectAnswers != nil {
				if parsed.CorrectAnswers == nil || *parsed.CorrectAnswers != *tc.conditions.CorrectAnswers {
					t.Errorf("CorrectAnswers mismatch: expected %v, got %v", tc.conditions.CorrectAnswers, parsed.CorrectAnswers)
				}
			}

			if tc.conditions.SpecificAnswer != nil {
				if parsed.SpecificAnswer == nil || *parsed.SpecificAnswer != *tc.conditions.SpecificAnswer {
					t.Errorf("SpecificAnswer mismatch: expected %v, got %v", tc.conditions.SpecificAnswer, parsed.SpecificAnswer)
				}
			}

			if len(tc.conditions.RequiredAchievements) > 0 {
				if len(parsed.RequiredAchievements) != len(tc.conditions.RequiredAchievements) {
					t.Errorf("RequiredAchievements length mismatch: expected %d, got %d",
						len(tc.conditions.RequiredAchievements), len(parsed.RequiredAchievements))
				}
			}
		})
	}
}
