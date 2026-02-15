package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ad/go-telegram-quest/internal/db"
	"github.com/ad/go-telegram-quest/internal/handlers"
	"github.com/ad/go-telegram-quest/internal/services"
	"github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	_ "github.com/joho/godotenv/autoload"
	_ "modernc.org/sqlite"
)

func main() {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is required")
	}

	adminIDStr := os.Getenv("ADMIN_ID")
	if adminIDStr == "" {
		log.Fatal("ADMIN_ID environment variable is required")
	}
	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil {
		log.Fatalf("Invalid ADMIN_ID: %v", err)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "quest.db"
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	if err := db.InitSchema(sqlDB); err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}

	dbQueue := db.NewDBQueue(sqlDB)
	defer dbQueue.Close()

	userRepo := db.NewUserRepository(dbQueue)
	stepRepo := db.NewStepRepository(dbQueue)
	progressRepo := db.NewProgressRepository(dbQueue)
	answerRepo := db.NewAnswerRepository(dbQueue)
	settingsRepo := db.NewSettingsRepository(dbQueue)
	chatStateRepo := db.NewChatStateRepository(dbQueue)
	adminMessagesRepo := db.NewAdminMessagesRepository(dbQueue)
	adminStateRepo := db.NewAdminStateRepository(dbQueue)
	achievementRepo := db.NewAchievementRepository(dbQueue)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	b, err := bot.New(botToken, bot.WithHTTPClient(15*time.Second, httpClient))
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Retry getMe with shorter timeout
	var botInfo *tgmodels.User
	for i := 0; i < 3; i++ {
		log.Printf("Attempting to connect to Telegram API (attempt %d/3)...", i+1)
		getMeCtx, getMeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		botInfo, err = b.GetMe(getMeCtx)
		getMeCancel()
		if err == nil {
			log.Printf("Successfully connected to Telegram API")
			break
		}
		log.Printf("Failed to get bot info (attempt %d/3): %v", i+1, err)
		if i < 2 {
			log.Printf("Retrying in 2 seconds...")
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		log.Fatalf("Failed to get bot info after 3 attempts: %v", err)
	}
	botUsername := botInfo.Username

	stickerPackRepo := db.NewStickerPackRepository(dbQueue)
	stickerService := services.NewStickerService(b, stickerPackRepo, botUsername, botToken)

	errorManager := services.NewErrorManager(b, adminID)
	stateResolver := services.NewStateResolver(stepRepo, progressRepo, userRepo)
	answerChecker := services.NewAnswerChecker(answerRepo, progressRepo, userRepo)
	msgManager := services.NewMessageManager(b, chatStateRepo, errorManager)
	statsService := services.NewStatisticsServiceWithAchievements(dbQueue, stepRepo, progressRepo, userRepo, achievementRepo)
	achievementEngine := services.NewAchievementEngine(achievementRepo, userRepo, progressRepo, stepRepo, dbQueue)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, chatStateRepo, achievementRepo, statsService, achievementEngine)
	questStateManager := services.NewQuestStateManager(settingsRepo)
	achievementNotifier := services.NewAchievementNotifier(b, achievementRepo, msgManager, stickerService)
	achievementService := services.NewAchievementService(achievementRepo, userRepo)
	retroactiveProcessor := services.NewRetroactiveProcessor(achievementEngine, achievementRepo, userRepo)
	groupChatVerifier := services.NewGroupChatVerifier(b, settingsRepo)

	handler := handlers.NewBotHandler(
		b,
		adminID,
		errorManager,
		stateResolver,
		answerChecker,
		msgManager,
		statsService,
		userRepo,
		stepRepo,
		progressRepo,
		answerRepo,
		settingsRepo,
		chatStateRepo,
		adminMessagesRepo,
		adminStateRepo,
		userManager,
		questStateManager,
		achievementEngine,
		achievementNotifier,
		achievementService,
		groupChatVerifier,
		dbPath,
	)

	b.RegisterHandlerMatchFunc(func(update *tgmodels.Update) bool {
		return true
	}, handler.HandleUpdate, logMiddleware)

	log.Printf("Bot started. Admin ID: %d, DB: %s", adminID, dbPath)

	// Process retroactive winner achievements
	go func() {
		// log.Printf("Starting retroactive processing for winner achievements...")
		for _, achievementKey := range []string{"winner_1", "winner_2", "winner_3", "hint_30", "writer"} {
			if _, err := retroactiveProcessor.ProcessAchievementSync(achievementKey, 50); err != nil {
				log.Printf("Failed to process retroactive achievement %s: %v", achievementKey, err)
			} else {
				// log.Printf("Successfully processed retroactive achievement: %s", achievementKey)
			}
		}
	}()

	b.Start(ctx)
}

func formatUser(u tgmodels.User) string {
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	if u.Username != "" {
		name += " @" + u.Username
	}
	return fmt.Sprintf("%s [%d]", name, u.ID)
}

func logMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *tgmodels.Update) {
		if update.Message != nil {
			log.Printf("[MSG] from=%s text=%q", formatUser(*update.Message.From), update.Message.Text)
		}
		if update.CallbackQuery != nil {
			log.Printf("[CALLBACK] from=%s data=%q", formatUser(update.CallbackQuery.From), update.CallbackQuery.Data)
		}
		next(ctx, b, update)
	}
}
