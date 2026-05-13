package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Postgres PostgresConfig
	Redis    RedisConfig
	Ollama   OllamaConfig
	Server   ServerConfig
	Worker   WorkerConfig
	Chunker  ChunkerConfig
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	MaxConns int32
	MinConns int32
}

type RedisConfig struct {
	Addr string
}

type OllamaConfig struct {
	BaseURL    string
	EmbedModel string
	LLMModel   string
}

type ServerConfig struct {
	Port string
}

type WorkerConfig struct {
	Concurrency int
}

type ChunkerConfig struct {
	Mode      string  // "fixed" | "semantic"
	Threshold float64 // semantic only, default 0.75
	MinSents  int     // semantic only, default 2
	MaxSents  int     // semantic only, default 20
}

func Load() (*Config, error) {
	maxConns, err := strconv.Atoi(getEnv("POSTGRES_MAX_CONNS", "10"))
	if err != nil {
		return nil, fmt.Errorf("invalid POSTGRES_MAX_CONNS: %w", err)
	}

	minConns, err := strconv.Atoi(getEnv("POSTGRES_MIN_CONNS", "2"))
	if err != nil {
		return nil, fmt.Errorf("invalid POSTGRES_MIN_CONNS: %w", err)
	}

	workerConcurrency, err := strconv.Atoi(getEnv("WORKER_CONCURRENCY", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid WORKER_CONCURRENCY: %w", err)
	}

	semanticThreshold, err := strconv.ParseFloat(getEnv("CHUNKER_SEMANTIC_THRESHOLD", "0.75"), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CHUNKER_SEMANTIC_THRESHOLD: %w", err)
	}

	semanticMinSents, err := strconv.Atoi(getEnv("CHUNKER_SEMANTIC_MIN_SENTS", "2"))
	if err != nil {
		return nil, fmt.Errorf("invalid CHUNKER_SEMANTIC_MIN_SENTS: %w", err)
	}

	semanticMaxSents, err := strconv.Atoi(getEnv("CHUNKER_SEMANTIC_MAX_SENTS", "20"))
	if err != nil {
		return nil, fmt.Errorf("invalid CHUNKER_SEMANTIC_MAX_SENTS: %w", err)
	}

	cfg := &Config{
		Postgres: PostgresConfig{
			Host:     requireEnv("POSTGRES_HOST"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     requireEnv("POSTGRES_USER"),
			Password: requireEnv("POSTGRES_PASSWORD"),
			DBName:   requireEnv("POSTGRES_DB"),
			MaxConns: int32(maxConns),
			MinConns: int32(minConns),
		},
		Redis: RedisConfig{
			Addr: getEnv("REDIS_ADDR", "localhost:6379"),
		},
		Ollama: OllamaConfig{
			BaseURL:    requireEnv("OLLAMA_BASE_URL"),
			EmbedModel: getEnv("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
			LLMModel:   getEnv("OLLAMA_LLM_MODEL", "qwen3:8b"),
		},
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
		},
		Worker: WorkerConfig{
			Concurrency: workerConcurrency,
		},

		Chunker: ChunkerConfig{
			Mode:      getEnv("CHUNKER_MODE", "fixed"),
			Threshold: semanticThreshold,
			MinSents:  semanticMinSents,
			MaxSents:  semanticMaxSents,
		},
	}

	return cfg, nil
}

func (c *PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.Host, c.Port, c.User, c.Password, c.DBName)
}

// requireEnv panics if the environment variable is not set or empty. It returns the value of the environment variable if it is set and not empty.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("environment variable not set or empty: " + key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
