package main

import (
	"context"
	"database/sql"
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	httpClient := &http.Client{
		Timeout: 65 * time.Second,
	}

	b, err := bot.New(botToken, bot.WithHTTPClient(60*time.Second, httpClient))
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	errorManager := services.NewErrorManager(b, adminID)
	stateResolver := services.NewStateResolver(stepRepo, progressRepo, userRepo)
	answerChecker := services.NewAnswerChecker(answerRepo, progressRepo, userRepo)
	msgManager := services.NewMessageManager(b, chatStateRepo, errorManager)
	statsService := services.NewStatisticsService(dbQueue, stepRepo, progressRepo, userRepo)
	userManager := services.NewUserManager(userRepo, stepRepo, progressRepo, answerRepo, statsService)

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
	)

	b.RegisterHandlerMatchFunc(func(update *tgmodels.Update) bool {
		return true
	}, handler.HandleUpdate, logMiddleware)

	log.Printf("Bot started. Admin ID: %d, DB: %s", adminID, dbPath)
	b.Start(ctx)
}

func logMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *tgmodels.Update) {
		if update.Message != nil {
			log.Printf("[MSG] from=%d text=%q", update.Message.From.ID, update.Message.Text)
		}
		if update.CallbackQuery != nil {
			log.Printf("[CALLBACK] from=%d data=%q", update.CallbackQuery.From.ID, update.CallbackQuery.Data)
		}
		next(ctx, b, update)
	}
}
