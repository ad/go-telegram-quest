package services

import (
	"fmt"
	"time"
)

// FormatDurationRussian formats a duration in Russian format
func FormatDurationRussian(d time.Duration) string {
	if d == 0 {
		return "0с"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	var parts []string

	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dд", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dч", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dм", minutes))
	}
	if seconds > 0 && days == 0 && hours == 0 {
		parts = append(parts, fmt.Sprintf("%dс", seconds))
	}

	if len(parts) == 0 {
		return "0с"
	}

	result := ""
	for i, part := range parts {
		if i > 0 {
			result += " "
		}
		result += part
	}

	return result
}

// FormatTimeAgo formats time as "X времени назад"
func FormatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	days := int(diff.Hours()) / 24
	hours := int(diff.Hours()) % 24
	minutes := int(diff.Minutes()) % 60

	if days > 0 {
		if days == 1 {
			return "1 день назад"
		} else if days < 5 {
			return fmt.Sprintf("%d дня назад", days)
		} else {
			return fmt.Sprintf("%d дней назад", days)
		}
	}

	if hours > 0 {
		if hours == 1 {
			return "1 час назад"
		} else if hours < 5 {
			return fmt.Sprintf("%d часа назад", hours)
		} else {
			return fmt.Sprintf("%d часов назад", hours)
		}
	}

	if minutes > 0 {
		if minutes == 1 {
			return "1 минуту назад"
		} else if minutes < 5 {
			return fmt.Sprintf("%d минуты назад", minutes)
		} else {
			return fmt.Sprintf("%d минут назад", minutes)
		}
	}

	return "только что"
}

// FormatDateTime formats time in Russian date/time format
func FormatDateTime(t time.Time) string {
	months := []string{
		"янв", "фев", "мар", "апр", "май", "июн",
		"июл", "авг", "сен", "окт", "ноя", "дек",
	}

	month := months[t.Month()-1]
	return fmt.Sprintf("%d %s %d, %02d:%02d", t.Day(), month, t.Year(), t.Hour(), t.Minute())
}

// FormatUserStatistics formats user statistics for display in admin messages
func FormatUserStatistics(stats *UserStatistics, isCompleted bool) string {
	if stats == nil {
		return ""
	}

	result := "📊 Статистика прохождения:\n\n"

	// Time section
	result += "⏱️ Время:\n"
	if stats.FirstAnswerTime != nil {
		result += fmt.Sprintf("• Первый ответ: %s\n", FormatDateTime(*stats.FirstAnswerTime))
	} else {
		result += "• Первый ответ: —\n"
	}

	if stats.LastAnswerTime != nil {
		result += fmt.Sprintf("• Последний ответ: %s\n", FormatDateTime(*stats.LastAnswerTime))
	} else {
		result += "• Последний ответ: —\n"
	}

	if stats.CompletionTime != nil {
		result += fmt.Sprintf("• Общее время: %s\n", FormatDurationRussian(*stats.CompletionTime))
	} else {
		result += "• Общее время: —\n"
	}

	result += "\n"

	// Accuracy section
	result += "🎯 Точность:\n"
	result += fmt.Sprintf("• Всего ответов: %d\n", stats.TotalAnswers)
	result += fmt.Sprintf("• Пройдено шагов: %d\n", stats.ApprovedSteps)
	result += fmt.Sprintf("• Точность: %d%%\n", stats.Accuracy)
	result += "\n"

	// Pace section
	result += "⚡ Темп:\n"
	if stats.AverageResponseTime != nil {
		result += fmt.Sprintf("• Среднее время ответа: %s\n", FormatDurationRussian(*stats.AverageResponseTime))
	} else {
		result += "• Среднее время ответа: —\n"
	}

	if stats.TimeOnCurrentStep != nil && !isCompleted {
		result += fmt.Sprintf("• На текущем шаге: %s\n", FormatDurationRussian(*stats.TimeOnCurrentStep))
	}

	result += "\n"

	// Errors section
	result += "❌ Ошибки по шагам:\n"
	if len(stats.StepAttempts) == 0 {
		result += "• Все шаги с первой попытки! 🎉\n"
	} else {
		for _, attempt := range stats.StepAttempts {
			result += fmt.Sprintf("• Шаг %d: %d попыток\n", attempt.StepOrder, attempt.Attempts)
		}
	}
	result += "\n"

	// Ranking section
	result += "🏆 Рейтинг:\n"
	medal := ""
	if stats.LeaderboardPosition == 1 {
		medal = "🥇 "
	} else if stats.LeaderboardPosition == 2 {
		medal = "🥈 "
	} else if stats.LeaderboardPosition == 3 {
		medal = "🥉 "
	}
	result += fmt.Sprintf("• Место: %s%d из %d\n", medal, stats.LeaderboardPosition, stats.TotalUsers)
	result += "\n"

	// Participation section
	result += "📅 Участие:\n"
	result += fmt.Sprintf("• Регистрация: %s\n", FormatDateTime(stats.RegistrationDate))
	result += fmt.Sprintf("• В квесте: %s\n", FormatTimeAgo(stats.RegistrationDate))

	if isCompleted {
		result += "• Статус: ✅ Квест завершён\n"
	}

	return result
}
