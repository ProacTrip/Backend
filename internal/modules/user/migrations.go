// Migration runner for the user module.
// Reads .sql files from the migrations directory, parses -- +migrate Up sections,
// and applies pending migrations tracked in a schema_migrations table.
package user

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const schemaMigrationsTable = `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)
`

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureSchemaMigrations(ctx, pool); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		if applied[name] {
			continue
		}

		upSQL, err := extractUpSection(name)
		if err != nil {
			return fmt.Errorf("parse migration %s: %w", name, err)
		}

		if err := applyMigration(ctx, pool, name, upSQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		slog.Info("migration applied", "module", "user", "file", name)
	}

	return nil
}

func ensureSchemaMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schemaMigrationsTable)
	return err
}

func getAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT filename FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

func extractUpSection(filename string) (string, error) {
	path := filepath.Join("migrations", filename)
	data, err := migrationFiles.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	idxUp := strings.Index(content, "-- +migrate Up")
	if idxUp == -1 {
		return "", fmt.Errorf("missing -- +migrate Up marker in %s", filename)
	}

	start := idxUp + len("-- +migrate Up")
	section := content[start:]

	idxDown := strings.Index(section, "-- +migrate Down")
	if idxDown == -1 {
		return "", fmt.Errorf("missing -- +migrate Down marker in %s", filename)
	}

	up := strings.TrimSpace(section[:idxDown])
	if up == "" {
		return "", fmt.Errorf("empty Up section in %s", filename)
	}

	return up, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, filename, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (filename) VALUES ($1)`,
		filename,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit(ctx)
}
