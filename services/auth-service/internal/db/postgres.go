package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}
	return pool, nil
}

// Migrate runs idempotent schema creation. Safe to call on every startup.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "pgcrypto";

		CREATE TABLE IF NOT EXISTS organizations (
			id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			name       TEXT        NOT NULL,
			slug       TEXT        UNIQUE NOT NULL,
			plan       TEXT        NOT NULL DEFAULT 'free',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS users (
			id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id        UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			email         TEXT        UNIQUE NOT NULL,
			password_hash TEXT        NOT NULL,
			status        TEXT        NOT NULL DEFAULT 'active',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS projects (
			id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id     UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
			name       TEXT        NOT NULL,
			region     TEXT        NOT NULL DEFAULT 'us-east-1',
			status     TEXT        NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS api_keys (
			id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name       TEXT        NOT NULL,
			key_hash   TEXT        NOT NULL,
			key_prefix TEXT        NOT NULL,
			scope      TEXT        NOT NULL DEFAULT 'full',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ
		);
	`)
	return err
}
