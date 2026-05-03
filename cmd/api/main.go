package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/valkey-io/valkey-go"

	"go-tasks-api/internal/auth"
	"go-tasks-api/internal/category"
	"go-tasks-api/internal/config"
	"go-tasks-api/internal/dailylog"
	"go-tasks-api/internal/db"
	"go-tasks-api/internal/metrics"
	"go-tasks-api/internal/middleware"
	"go-tasks-api/internal/occurrence"
	"go-tasks-api/internal/shared/logger"
	"go-tasks-api/internal/task"
)

func main() {
	// Parse command-line flags. --version / -v prints version info and exits.
	// Flag parsing is the first action in main so that a version query
	// never touches config, database, Valkey, or any goroutine.
	shouldExit, code := handleFlags(os.Args[1:], os.Stdout)
	if shouldExit {
		os.Exit(code)
	}

	// Load configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialise logger with configured level.
	appLogger := logger.NewLogger(cfg.Log.Level)

	appLogger.LogInfo("starting application")

	// Initialise database with configuration.
	database, err := db.New(&cfg.Database, appLogger)
	if err != nil {
		appLogger.LogError(err, nil)
		os.Exit(1)
	}

	// Initialise Valkey client.
	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.Valkey.URL},
	})
	if err != nil {
		appLogger.LogError(errors.New("failed to connect to Valkey"), err)
		os.Exit(1)
	}
	defer valkeyClient.Close()

	// Initialise Prometheus metrics.
	appMetrics := metrics.New()

	// Create router.
	r := chi.NewRouter()

	// Global middleware.
	r.Use(chimw.RequestID)
	r.Use(middleware.RequestLogger(appLogger)) // Bind request ID to all log entries.
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.SecurityHeaders) // Security headers on all responses.
	r.Use(middleware.CORS)            // Cross-origin resource sharing.
	r.Use(appMetrics.Middleware)      // Prometheus request instrumentation.
	r.Use(chimw.Timeout(60 * time.Second))
	// Limit request bodies to 1MB to prevent memory exhaustion.
	// Adjust per-route if you need larger uploads on specific endpoints.
	r.Use(chimw.RequestSize(1 * 1024 * 1024))

	// Prometheus metrics endpoint — no auth required.
	// WARNING: In production, restrict access to /metrics via network policy, reverse proxy,
	// or move to a separate internal port. Exposed metrics can reveal system internals.
	r.Handle("/metrics", metrics.Handler())

	// Health check — no auth required; used by load balancers and orchestration.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := database.HealthCheck(r.Context()); err != nil {
			appLogger.LogError(errors.New("health check failed"), err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})

	// Wire up auth domain.
	authRepo := auth.NewAuthRepository(database.SQL(), appLogger)
	authService, err := auth.NewAuthService(authRepo, appLogger, &cfg.JWT, valkeyClient)
	if err != nil {
		appLogger.LogError(errors.New("failed to initialise auth service"), err)
		os.Exit(1)
	}
	authHandler := auth.NewAuthHandler(authService, appLogger)
	authMiddleware := auth.NewAuthMiddleware(authService, appLogger)

	// Wire up category domain.
	categoryRepo := category.NewCategoryRepository(database.SQL(), appLogger)
	categoryService := category.NewCategoryService(categoryRepo, appLogger)
	categoryHandler := category.NewCategoryHandler(categoryService, appLogger)

	// Wire up task domain.
	taskRepo := task.NewTaskRepository(database.SQL(), appLogger)
	taskService := task.NewTaskService(taskRepo, appLogger)
	taskHandler := task.NewTaskHandler(taskService, appLogger)

	// Wire up occurrence domain.
	occurrenceRepo := occurrence.NewOccurrenceRepository(database.SQL(), appLogger)
	occurrenceService := occurrence.NewOccurrenceService(occurrenceRepo, appLogger)
	occurrenceHandler := occurrence.NewOccurrenceHandler(occurrenceService, appLogger)

	// Wire up daily log domain.
	dailylogRepo := dailylog.NewDailyLogRepository(database.SQL(), appLogger)
	dailylogService := dailylog.NewDailyLogService(dailylogRepo, appLogger)
	dailylogHandler := dailylog.NewDailyLogHandler(dailylogService, appLogger)

	// Bind shutdown signals to a context. signal.NotifyContext registers the
	// handler before any goroutine reads from it, so signals always reach the
	// shutdown path and a single channel is not raced between consumers.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Start background cleanup of expired refresh tokens.
	// The goroutine watches shutdownCtx.Done() — closing a context broadcasts to
	// every reader without contending for a value, unlike a signal channel.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := authRepo.CleanExpiredTokens(context.Background()); err != nil {
					appLogger.LogError(errors.New("failed to clean expired tokens"), err)
				}
			case <-shutdownCtx.Done():
				return
			}
		}
	}()

	// Auth endpoints — no auth middleware required.
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
	})

	// API routes — auth required.
	r.Route("/api/v1", func(r chi.Router) {
		// Apply auth middleware to all routes in this group.
		r.Use(authMiddleware.Handler)

		// Category routes.
		r.Get("/categories", categoryHandler.ListCategories)
		r.Post("/categories", categoryHandler.CreateCategory)
		r.Get("/categories/inactive", categoryHandler.ListInactiveCategories)
		r.Post("/categories/bulk-delete", categoryHandler.BulkDeleteCategories)
		r.Post("/categories/bulk-permanent-delete", categoryHandler.BulkPermanentDeleteCategories)
		r.Get("/categories/{id}", categoryHandler.GetCategory)
		r.Put("/categories/{id}", categoryHandler.UpdateCategory)
		r.Delete("/categories/{id}", categoryHandler.DeleteCategory)
		r.Delete("/categories/{id}/permanent", categoryHandler.PermanentDeleteCategory)
		r.Post("/categories/{id}/reactivate", categoryHandler.ReactivateCategory)

		// Task routes.
		r.Get("/tasks", taskHandler.ListTasks)
		r.Post("/tasks", taskHandler.CreateTask)
		r.Get("/tasks/inactive", taskHandler.ListInactiveTasks)
		r.Post("/tasks/bulk-delete", taskHandler.BulkDeleteTasks)
		r.Post("/tasks/bulk-permanent-delete", taskHandler.BulkPermanentDeleteTasks)
		r.Get("/tasks/{id}", taskHandler.GetTask)
		r.Put("/tasks/{id}", taskHandler.UpdateTask)
		r.Delete("/tasks/{id}", taskHandler.DeleteTask)
		r.Delete("/tasks/{id}/permanent", taskHandler.PermanentDeleteTask)
		r.Post("/tasks/{id}/reactivate", taskHandler.ReactivateTask)

		// Occurrence routes.
		r.Get("/occurrences", occurrenceHandler.ListOccurrences)
		r.Post("/occurrences/bulk-delete-answers", occurrenceHandler.BulkDeleteAnswers)
		r.Post("/occurrences/{id}/suppress", occurrenceHandler.SuppressOccurrence)
		r.Post("/occurrences/{id}/unsuppress", occurrenceHandler.UnsuppressOccurrence)
		r.Post("/occurrences/{id}/answer", occurrenceHandler.SubmitAnswer)

		// Daily log routes.
		r.Get("/daily-logs", dailylogHandler.ListDailyLogs)
		r.Post("/daily-logs", dailylogHandler.CreateDailyLog)
		r.Get("/daily-logs/inactive", dailylogHandler.ListInactiveDailyLogs)
		r.Post("/daily-logs/bulk-delete", dailylogHandler.BulkDeleteDailyLogs)
		r.Post("/daily-logs/bulk-permanent-delete", dailylogHandler.BulkPermanentDeleteDailyLogs)
		r.Put("/daily-logs/{id}", dailylogHandler.UpdateDailyLog)
		r.Delete("/daily-logs/{id}", dailylogHandler.DeleteDailyLog)
		r.Delete("/daily-logs/{id}/permanent", dailylogHandler.PermanentDeleteDailyLog)
		r.Post("/daily-logs/{id}/reactivate", dailylogHandler.ReactivateDailyLog)
	})

	server := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		appLogger.LogInfo("api server listening", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.LogError(errors.New("server error"), err)
			os.Exit(1)
		}
	}()

	appLogger.LogInfo("application ready",
		"port", cfg.Server.Port,
		"log_level", cfg.Log.Level,
		"db_host", cfg.Database.Host,
		"db_name", cfg.Database.Name,
		"valkey_url", cfg.Valkey.URL,
	)

	// Block until a shutdown signal arrives.
	<-shutdownCtx.Done()
	stopSignals() // restore default signal handling for any second SIGINT

	appLogger.LogInfo("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	shutdownStart := time.Now()
	if err := server.Shutdown(ctx); err != nil {
		appLogger.LogError(errors.New("server shutdown error"), err)
	} else {
		appLogger.LogInfo("http server shutdown complete", "duration_ms", time.Since(shutdownStart).Milliseconds())
	}

	// Close repositories (releases prepared statements) before closing connections.
	appLogger.LogInfo("closing repositories")

	start := time.Now()
	if err := authRepo.Close(); err != nil {
		appLogger.LogError(errors.New("auth repository close error"), err)
	} else {
		appLogger.LogInfo("auth repository closed", "duration_ms", time.Since(start).Milliseconds())
	}

	start = time.Now()
	if err := categoryRepo.Close(); err != nil {
		appLogger.LogError(errors.New("category repository close error"), err)
	} else {
		appLogger.LogInfo("category repository closed", "duration_ms", time.Since(start).Milliseconds())
	}

	start = time.Now()
	if err := taskRepo.Close(); err != nil {
		appLogger.LogError(errors.New("task repository close error"), err)
	} else {
		appLogger.LogInfo("task repository closed", "duration_ms", time.Since(start).Milliseconds())
	}

	start = time.Now()
	if err := occurrenceRepo.Close(); err != nil {
		appLogger.LogError(errors.New("occurrence repository close error"), err)
	} else {
		appLogger.LogInfo("occurrence repository closed", "duration_ms", time.Since(start).Milliseconds())
	}

	start = time.Now()
	if err := dailylogRepo.Close(); err != nil {
		appLogger.LogError(errors.New("dailylog repository close error"), err)
	} else {
		appLogger.LogInfo("dailylog repository closed", "duration_ms", time.Since(start).Milliseconds())
	}

	appLogger.LogInfo("closing database connection")
	start = time.Now()
	if err := database.Close(); err != nil {
		appLogger.LogError(errors.New("database close error"), err)
	} else {
		appLogger.LogInfo("database connection closed", "duration_ms", time.Since(start).Milliseconds())
	}

	appLogger.LogInfo("server stopped")
}
