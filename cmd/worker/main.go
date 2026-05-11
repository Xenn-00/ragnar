package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Xenn-00/ragnar/internal/cache"
	"github.com/Xenn-00/ragnar/internal/config"
	"github.com/Xenn-00/ragnar/internal/ingestion"
	"github.com/Xenn-00/ragnar/internal/pipeline"
	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/Xenn-00/ragnar/pkg/provider/embedder"
	"github.com/Xenn-00/ragnar/pkg/task"
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

	// -- stores ---
	documentStore := store.NewDocumentStore(db)
	chunkStore := store.NewChunkStore(db)

	// -- pipelines ---
	parser := ingestion.NewParser()
	chunker := ingestion.NewChunker(0, 0) // 0 = using default values

	ingestionPipeline := pipeline.NewIngestionPipeline(parser, chunker, cachedEmbedder, documentStore, chunkStore, log)

	// -- asynq server ---
	srv := asynq.NewServer(asynq.RedisClientOpt{Addr: cfg.Redis.Addr}, asynq.Config{
		Concurrency: cfg.Worker.Concurrency,
		// retry 3x with exponential backoff
		RetryDelayFunc: asynq.DefaultRetryDelayFunc,
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			log.ErrorContext(ctx, "task failed", "type", task.Type(), "error", err)
		}),
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(task.TypeIngestDocument, makeIngestHandler(ingestionPipeline, log))

	// -- graceful shutdown ---

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("worker starting", "concurrency", cfg.Worker.Concurrency, "queue", task.TypeIngestDocument)

		if err := srv.Run(mux); err != nil {
			log.Error("worker error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	log.Info("shutting down worker...")
	srv.Shutdown()
	log.Info("worker exited")
}

// makeIngestHandler return asynq.Handlerfunc for task document:ingest
// Seperated from main in order to testable and doesn't make main() too long
func makeIngestHandler(p *pipeline.IngestionPipeline, log *slog.Logger) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload task.IngestPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			// return nil so that asynq not retry - payload corrupt could not be fixed with retry
			log.ErrorContext(ctx, "failed to unmarshal ingest payload", "error", err)
			return nil
		}

		if err := p.Run(ctx, payload.DocumentID, payload.Filename, payload.Data, payload.MimeType); err != nil {
			// return error so that asynq would retry
			return fmt.Errorf("ingestion pipeline: %w", err)
		}
		log.Info("payload data length", "length", len(payload.Data))
		log.Info("payload data preview", "preview", string(payload.Data[:min(len(payload.Data), 100)]))

		return nil
	}
}
