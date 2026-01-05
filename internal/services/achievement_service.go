package services

import (
	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type AchievementStatistics struct {
	TotalAchievements      int
	TotalUserAchievements  int
	AchievementsByCategory map[models.AchievementCategory]int
	PopularAchievements    []AchievementPopularity
	TotalUsers             int
}

type AchievementPopularity struct {
	Achievement *models.Achievement
	UserCount   int
	Percentage  float64
}

type UserAchievementDetails struct {
	Achievement *models.Achievement
	EarnedAt    string
}

type UserAchievementSummary struct {
	TotalCount             int
	AchievementsByCategory map[models.AchievementCategory][]*UserAchievementDetails
}

type AchievementService struct {
	achievementRepo *db.AchievementRepository
	userRepo        *db.UserRepository
}

func NewAchievementService(achievementRepo *db.AchievementRepository, userRepo *db.UserRepository) *AchievementService {
	return &AchievementService{
		achievementRepo: achievementRepo,
		userRepo:        userRepo,
	}
}

func (s *AchievementService) CreateAchievement(achievement *models.Achievement) error {
	return s.achievementRepo.Create(achievement)
}

func (s *AchievementService) UpdateAchievement(achievement *models.Achievement) error {
	return s.achievementRepo.Update(achievement)
}

func (s *AchievementService) GetAllAchievements() ([]*models.Achievement, error) {
	return s.achievementRepo.GetAll()
}

func (s *AchievementService) GetActiveAchievements() ([]*models.Achievement, error) {
	return s.achievementRepo.GetActive()
}

func (s *AchievementService) GetAchievementByKey(key string) (*models.Achievement, error) {
	return s.achievementRepo.GetByKey(key)
}

func (s *AchievementService) GetUserAchievements(userID int64) ([]*models.UserAchievement, error) {
	return s.achievementRepo.GetUserAchievements(userID)
}

func (s *AchievementService) GetUserAchievementsByCategory(userID int64, category models.AchievementCategory) ([]*models.UserAchievement, error) {
	return s.achievementRepo.GetUserAchievementsByCategory(userID, category)
}

func (s *AchievementService) GetUserAchievementCount(userID int64) (int, error) {
	return s.achievementRepo.CountUserAchievements(userID)
}

func (s *AchievementService) GetUserAchievementSummary(userID int64) (*UserAchievementSummary, error) {
	userAchievements, err := s.achievementRepo.GetUserAchievements(userID)
	if err != nil {
		return nil, err
	}

	summary := &UserAchievementSummary{
		TotalCount:             len(userAchievements),
		AchievementsByCategory: make(map[models.AchievementCategory][]*UserAchievementDetails),
	}

	for _, ua := range userAchievements {
		achievement, err := s.achievementRepo.GetByID(ua.AchievementID)
		if err != nil {
			continue
		}

		details := &UserAchievementDetails{
			Achievement: achievement,
			EarnedAt:    ua.EarnedAt.Format("02.01.2006 15:04"),
		}

		summary.AchievementsByCategory[achievement.Category] = append(
			summary.AchievementsByCategory[achievement.Category],
			details,
		)
	}

	return summary, nil
}

func (s *AchievementService) GetAchievementStatistics() (*AchievementStatistics, error) {
	allAchievements, err := s.achievementRepo.GetAll()
	if err != nil {
		return nil, err
	}

	achievementStats, err := s.achievementRepo.GetAchievementStats()
	if err != nil {
		return nil, err
	}

	allUsers, err := s.userRepo.GetAll()
	if err != nil {
		return nil, err
	}
	totalUsers := len(allUsers)

	stats := &AchievementStatistics{
		TotalAchievements:      len(allAchievements),
		TotalUserAchievements:  0,
		AchievementsByCategory: make(map[models.AchievementCategory]int),
		PopularAchievements:    make([]AchievementPopularity, 0),
		TotalUsers:             totalUsers,
	}

	for _, achievement := range allAchievements {
		stats.AchievementsByCategory[achievement.Category]++
		userCount := achievementStats[achievement.Key]
		stats.TotalUserAchievements += userCount

		percentage := 0.0
		if totalUsers > 0 {
			percentage = float64(userCount) / float64(totalUsers) * 100
		}

		stats.PopularAchievements = append(stats.PopularAchievements, AchievementPopularity{
			Achievement: achievement,
			UserCount:   userCount,
			Percentage:  percentage,
		})
	}

	s.sortPopularAchievements(stats.PopularAchievements)

	return stats, nil
}

func (s *AchievementService) sortPopularAchievements(achievements []AchievementPopularity) {
	for i := 0; i < len(achievements)-1; i++ {
		for j := i + 1; j < len(achievements); j++ {
			if achievements[j].UserCount > achievements[i].UserCount {
				achievements[i], achievements[j] = achievements[j], achievements[i]
			}
		}
	}
}

func (s *AchievementService) GetUsersWithMostAchievements(limit int) ([]UserAchievementRanking, error) {
	userCounts, err := s.achievementRepo.GetUsersWithAchievementCount()
	if err != nil {
		return nil, err
	}

	rankings := make([]UserAchievementRanking, 0, len(userCounts))
	for userID, count := range userCounts {
		user, err := s.userRepo.GetByID(userID)
		if err != nil {
			continue
		}
		rankings = append(rankings, UserAchievementRanking{
			User:             user,
			AchievementCount: count,
		})
	}

	for i := 0; i < len(rankings)-1; i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[j].AchievementCount > rankings[i].AchievementCount {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	if limit > 0 && len(rankings) > limit {
		rankings = rankings[:limit]
	}

	return rankings, nil
}

type UserAchievementRanking struct {
	User             *models.User
	AchievementCount int
}

func (s *AchievementService) HasUserAchievement(userID int64, achievementKey string) (bool, error) {
	return s.achievementRepo.HasUserAchievement(userID, achievementKey)
}

func (s *AchievementService) GetAchievementHolders(achievementKey string) ([]int64, error) {
	return s.achievementRepo.GetAchievementHolders(achievementKey)
}
