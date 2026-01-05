package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/models"
)

type RetroactiveProgress struct {
	TotalUsers     int
	ProcessedUsers int
	AwardedCount   int
	ErrorCount     int
	StartTime      time.Time
	EndTime        *time.Time
	IsRunning      bool
	CurrentBatch   int
	TotalBatches   int
	Errors         []string
}

type RetroactiveProcessor struct {
	achievementEngine *AchievementEngine
	achievementRepo   *db.AchievementRepository
	userRepo          *db.UserRepository

	mu       sync.RWMutex
	progress map[string]*RetroactiveProgress
	cancel   map[string]context.CancelFunc
}

func NewRetroactiveProcessor(
	achievementEngine *AchievementEngine,
	achievementRepo *db.AchievementRepository,
	userRepo *db.UserRepository,
) *RetroactiveProcessor {
	return &RetroactiveProcessor{
		achievementEngine: achievementEngine,
		achievementRepo:   achievementRepo,
		userRepo:          userRepo,
		progress:          make(map[string]*RetroactiveProgress),
		cancel:            make(map[string]context.CancelFunc),
	}
}

const DefaultBatchSize = 50

func (p *RetroactiveProcessor) ProcessAchievementAsync(achievementKey string, batchSize int) error {
	p.mu.Lock()
	if prog, exists := p.progress[achievementKey]; exists && prog.IsRunning {
		p.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel[achievementKey] = cancel

	p.progress[achievementKey] = &RetroactiveProgress{
		StartTime: time.Now(),
		IsRunning: true,
	}
	p.mu.Unlock()

	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	go p.processInBackground(ctx, achievementKey, batchSize)

	return nil
}

func (p *RetroactiveProcessor) processInBackground(ctx context.Context, achievementKey string, batchSize int) {
	defer func() {
		p.mu.Lock()
		if prog, exists := p.progress[achievementKey]; exists {
			prog.IsRunning = false
			now := time.Now()
			prog.EndTime = &now
		}
		delete(p.cancel, achievementKey)
		p.mu.Unlock()
	}()

	achievement, err := p.achievementRepo.GetByKey(achievementKey)
	if err != nil {
		p.recordError(achievementKey, "Failed to get achievement: "+err.Error())
		return
	}

	users, err := p.userRepo.GetAll()
	if err != nil {
		p.recordError(achievementKey, "Failed to get users: "+err.Error())
		return
	}

	p.mu.Lock()
	prog := p.progress[achievementKey]
	prog.TotalUsers = len(users)
	prog.TotalBatches = (len(users) + batchSize - 1) / batchSize
	p.mu.Unlock()

	for i := 0; i < len(users); i += batchSize {
		select {
		case <-ctx.Done():
			log.Printf("[RETROACTIVE_PROCESSOR] Processing cancelled for achievement %s", achievementKey)
			return
		default:
		}

		end := i + batchSize
		if end > len(users) {
			end = len(users)
		}
		batch := users[i:end]

		p.mu.Lock()
		prog.CurrentBatch = (i / batchSize) + 1
		p.mu.Unlock()

		p.processBatch(ctx, achievementKey, achievement, batch)

		if end < len(users) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	log.Printf("[RETROACTIVE_PROCESSOR] Completed processing achievement %s: %d users processed, %d awarded",
		achievementKey, p.progress[achievementKey].ProcessedUsers, p.progress[achievementKey].AwardedCount)
}

func (p *RetroactiveProcessor) processBatch(ctx context.Context, achievementKey string, achievement *models.Achievement, users []*models.User) {
	for _, user := range users {
		select {
		case <-ctx.Done():
			return
		default:
		}

		awarded, err := p.processUser(user.ID, achievementKey, achievement)

		p.mu.Lock()
		prog := p.progress[achievementKey]
		prog.ProcessedUsers++
		if err != nil {
			prog.ErrorCount++
			if len(prog.Errors) < 100 {
				prog.Errors = append(prog.Errors, err.Error())
			}
		} else if awarded {
			prog.AwardedCount++
		}
		p.mu.Unlock()
	}
}

func (p *RetroactiveProcessor) processUser(userID int64, achievementKey string, achievement *models.Achievement) (bool, error) {
	hasAchievement, err := p.achievementRepo.HasUserAchievement(userID, achievementKey)
	if err != nil {
		return false, err
	}
	if hasAchievement {
		return false, nil
	}

	switch achievement.Category {
	case models.CategoryCompletion:
		qualifies, earnedAt, err := p.achievementEngine.EvaluateCompletionConditionsWithTimestamp(userID, achievement)
		if err != nil {
			return false, err
		}
		if qualifies {
			err = p.achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, true)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	case models.CategoryComposite:
		qualifies, earnedAt, err := p.achievementEngine.EvaluateCompositeConditionsWithTimestamp(userID, achievement)
		if err != nil {
			return false, err
		}
		if qualifies {
			err = p.achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, true)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	default:
		qualifies, earnedAt, err := p.achievementEngine.evaluateConditionsWithTimestamp(userID, achievement)
		if err != nil {
			return false, err
		}
		if qualifies {
			err = p.achievementRepo.AssignToUser(userID, achievement.ID, earnedAt, true)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}

	return false, nil
}

func (p *RetroactiveProcessor) recordError(achievementKey string, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if prog, exists := p.progress[achievementKey]; exists {
		prog.ErrorCount++
		if len(prog.Errors) < 100 {
			prog.Errors = append(prog.Errors, errMsg)
		}
	}
	log.Printf("[RETROACTIVE_PROCESSOR] Error for achievement %s: %s", achievementKey, errMsg)
}

func (p *RetroactiveProcessor) GetProgress(achievementKey string) *RetroactiveProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if prog, exists := p.progress[achievementKey]; exists {
		copy := *prog
		copy.Errors = make([]string, len(prog.Errors))
		for i, e := range prog.Errors {
			copy.Errors[i] = e
		}
		return &copy
	}
	return nil
}

func (p *RetroactiveProcessor) CancelProcessing(achievementKey string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cancel, exists := p.cancel[achievementKey]; exists {
		cancel()
		return true
	}
	return false
}

func (p *RetroactiveProcessor) IsProcessing(achievementKey string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if prog, exists := p.progress[achievementKey]; exists {
		return prog.IsRunning
	}
	return false
}

func (p *RetroactiveProcessor) ProcessAllAchievementsAsync(batchSize int) error {
	achievements, err := p.achievementRepo.GetActive()
	if err != nil {
		return err
	}

	for _, achievement := range achievements {
		if achievement.IsUnique {
			continue
		}
		if err := p.ProcessAchievementAsync(achievement.Key, batchSize); err != nil {
			log.Printf("[RETROACTIVE_PROCESSOR] Failed to start processing for %s: %v", achievement.Key, err)
		}
	}

	return nil
}

func (p *RetroactiveProcessor) GetAllProgress() map[string]*RetroactiveProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*RetroactiveProgress)
	for key, prog := range p.progress {
		copy := *prog
		copy.Errors = make([]string, len(prog.Errors))
		for i, e := range prog.Errors {
			copy.Errors[i] = e
		}
		result[key] = &copy
	}
	return result
}

func (p *RetroactiveProcessor) ProcessAchievementSync(achievementKey string, batchSize int) (*RetroactiveProgress, error) {
	p.mu.Lock()
	if prog, exists := p.progress[achievementKey]; exists && prog.IsRunning {
		p.mu.Unlock()
		return nil, nil
	}

	p.progress[achievementKey] = &RetroactiveProgress{
		StartTime: time.Now(),
		IsRunning: true,
	}
	p.mu.Unlock()

	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	ctx := context.Background()
	p.processInBackground(ctx, achievementKey, batchSize)

	return p.GetProgress(achievementKey), nil
}
