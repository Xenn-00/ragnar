package store

import (
	"context"
	"fmt"

	"github.com/Xenn-00/ragnar/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a thin wrapper around pgxpool.Pool
// All other package that require DB would use this struct instead of pgxpool.Pool directly
// So that we can easily swap out the underlying database implementation if needed or mock it for testing
type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(ctx context.Context, cfg *config.PostgresConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())

	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("crete pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}
