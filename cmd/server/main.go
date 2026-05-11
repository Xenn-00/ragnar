package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/Xenn-00/ragnar/internal/metrics"

	"github.com/Xenn-00/ragnar/cmd/server/handler"
	"github.com/Xenn-00/ragnar/internal/cache"
	"github.com/Xenn-00/ragnar/internal/config"
	"github.com/Xenn-00/ragnar/internal/generation"
	"github.com/Xenn-00/ragnar/internal/pipeline"
	"github.com/Xenn-00/ragnar/internal/retrieval"
	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/Xenn-00/ragnar/pkg/provider/embedder"
	"github.com/Xenn-00/ragnar/pkg/provider/llm"
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
)

func main() {
	// load .env (ignoring error when file is missing)
	_ = godotenv.Load()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", "error", "err")
		os.Exit(1)
	}

	ctx := context.Background()

	// -- infrastructure ----
	db, err := store.NewDB(ctx, &cfg.Postgres)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}

	defer db.Close()
	log.Info("postgres connected")

	embeddingCache := cache.NewEmbeddingCache(&cfg.Redis)
	if err := embeddingCache.Ping(ctx); err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer embeddingCache.Close()
	log.Info("redis connected")

	// -- providers ---
	ollamaEmbedder := embedder.NewOllama(cfg.Ollama.BaseURL, cfg.Ollama.EmbedModel)
	cachedEmbedder := embedder.NewCached(ollamaEmbedder, embeddingCache)

	ollamaLLM := llm.NewOllama(cfg.Ollama.BaseURL, cfg.Ollama.LLMModel)

	// -- stores ---
	documentStore := store.NewDocumentStore(db)
	chunkStore := store.NewChunkStore(db)

	// -- retriever and generation ---
	retriever := retrieval.NewRetriever(cachedEmbedder, chunkStore)
	generator := generation.NewGenerator(ollamaLLM)

	queryPipeline := pipeline.NewQueryPipeline(retriever, generator, log)

	// -- asynq client (for enqueue jobs from HTTP handler) ---
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.Redis.Addr})
	defer asynqClient.Close()

	// -- HTTP server ---

	app := fiber.New(fiber.Config{
		// return error as JSON, not HTML default fiber
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${method} ${path} ${status} ${latency}\n",
	}))

	// -- metrics ---
	fiberPrometheus := fiberprometheus.NewWithDefaultRegistry("rag_engine")
	fiberPrometheus.RegisterAt(app, "/metrics")
	app.Use(fiberPrometheus.Middleware)

	// -- routes ---
	docHandler := handler.NewDocumentHandler(documentStore, asynqClient, log)
	queryHandler := handler.NewQueryHandler(queryPipeline, log)

	v1 := app.Group("/v1")
	v1.Post("/documents", docHandler.Upload)
	v1.Get("/documents/:id", docHandler.GetByID)
	v1.Delete("/documents/:id", docHandler.Delete)

	v1.Post("/query", queryHandler.Query)
	v1.Get("/query/stream", queryHandler.Stream)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// -- graceful shutdown ---

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := ":" + cfg.Server.Port
		log.Info("server starting", "addr", addr)
		if err := app.Listen(addr); err != nil {
			log.Error("server error", "error", err)
		}
	}()

	<-quit
	log.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", "error", err)
	}

	log.Info("server exited")
}
