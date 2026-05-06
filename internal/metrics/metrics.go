package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// -- Ingestion ---
	IngestionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_ingestion_duration_seconds",
		Help:    "Duration of full ingetsion pipeline per document",
		Buckets: prometheus.DefBuckets,
	})

	IngestionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rag_ingestion_total",
		Help: "Total number of ingestion jobs by status",
	}, []string{"status"}) // status = success | failed

	ChunksPerDocument = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_chunks_per_document",
		Help:    "Number of chunks produced per document",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
	})

	// -- Query ---
	QueryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_query_duration_seconds",
		Help:    "Total duration of full query pipeline",
		Buckets: prometheus.DefBuckets,
	})

	RetrievalDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_retrieval_duration_seconds",
		Help:    "Duration of retrieval step (embed + search + rerank)",
		Buckets: prometheus.DefBuckets,
	})

	GenerationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_generation_duration_seconds",
		Help:    "Duration of LLM generation step",
		Buckets: prometheus.DefBuckets,
	})

	// -- Cache ---

	EmbeddingCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rag_embedding_cache_hits_total",
		Help: "Total number of embedding cache hits",
	})

	EmbeddingCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rag_embedding_cache_misses_total",
		Help: "Total number of embedding cache misses",
	})
)
