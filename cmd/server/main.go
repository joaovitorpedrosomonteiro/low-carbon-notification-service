package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/application"
	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/infrastructure/email"
	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/infrastructure/postgres"
	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/infrastructure/pubsub"
	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/infrastructure/push"
	redisInfra "github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/infrastructure/redis"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/notification?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Unable to ping database: %v", err)
	}

	if err := runMigrations(pool); err != nil {
		log.Printf("Warning: migration error: %v", err)
	}

	var redisClient interface {
		SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
		Close() error
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		client, err := redisInfra.NewRedisClient(ctx)
		if err != nil {
			log.Printf("Warning: Redis connection failed: %v", err)
			redisClient = redisInfra.NewMockRedisClient()
		} else {
			redisClient = client
		}
	} else {
		redisClient = redisInfra.NewMockRedisClient()
	}

	notificationRepo := postgres.NewNotificationRepository(pool)
	deviceTokenRepo := postgres.NewDeviceTokenRepository(pool)

	var emailSender application.EmailSender
	gmailCreds := os.Getenv("GMAIL_SERVICE_ACCOUNT_JSON")
	if gmailCreds != "" {
		sender, err := email.NewGmailSender(ctx)
		if err != nil {
			log.Printf("Warning: Gmail sender init failed: %v, using mock", err)
			emailSender = email.NewMockEmailSender()
		} else {
			emailSender = sender
		}
	} else {
		smtpHost := os.Getenv("SMTP_HOST")
		if smtpHost != "" {
			emailSender = email.NewSMTPEmailSender()
		} else {
			emailSender = email.NewMockEmailSender()
		}
	}

	var pushSender application.PushSender
	if os.Getenv("EXPO_PUSH_ENABLED") == "true" {
		pushSender = push.NewExpoPushClient()
	} else {
		pushSender = push.NewMockPushClient()
	}

	handler := application.NewEventHandler(notificationRepo, deviceTokenRepo, emailSender, pushSender)

	var deduplicator pubsub.Deduplicator
	if os.Getenv("PUBSUB_EMULATOR_HOST") != "" || os.Getenv("GCP_PROJECT_ID") != "" {
		deduplicator = pubsub.NewRedisDeduplicator(redisClient)
	} else {
		deduplicator = pubsub.NewMockDeduplicator()
	}

	subscriber, err := pubsub.NewSubscriber(ctx, handler, deduplicator)
	if err != nil {
		log.Printf("Warning: Pub/Sub subscriber init failed: %v", err)
		log.Println("Running without Pub/Sub subscriber (local development mode)")
	} else {
		if err := subscriber.Start(ctx); err != nil {
			log.Printf("Warning: Pub/Sub subscriber start failed: %v", err)
		}
		defer subscriber.Close()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	loggingMux := loggingMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: loggingMux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		log.Println("Shutting down gracefully...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Notification Service starting on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func runMigrations(pool *pgxpool.Pool) error {
	migration := `
	CREATE TABLE IF NOT EXISTS notifications (
		id VARCHAR(64) PRIMARY KEY,
		type VARCHAR(64) NOT NULL,
		recipient_user_id VARCHAR(64) NOT NULL,
		recipient_email VARCHAR(255) NOT NULL,
		subject VARCHAR(512) NOT NULL,
		body TEXT NOT NULL,
		channel VARCHAR(16) NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(recipient_user_id);
	CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
	CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at);

	CREATE TABLE IF NOT EXISTS device_tokens (
		user_id VARCHAR(64) NOT NULL,
		token VARCHAR(512) NOT NULL,
		platform VARCHAR(16) NOT NULL,
		registered_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		PRIMARY KEY (user_id, token)
	);

	CREATE INDEX IF NOT EXISTS idx_device_tokens_user ON device_tokens(user_id);
	`
	_, err := pool.Exec(context.Background(), migration)
	return err
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Completed: %s %s in %v", r.Method, r.URL.Path, time.Since(start))
	})
}
