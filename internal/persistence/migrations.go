package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// migration is a single schema migration. Migrations are applied in
// ascending version order. Each migration runs in its own transaction.
type migration struct {
	version int
	name    string
	sql     string
}

// migrations is the list of all schema migrations, in order.
// Never reorder or rewrite past migrations — append new ones only.
var migrations = []migration{
	{
		version: 1,
		name:    "initial schema",
		sql: `
			CREATE TABLE schema_migrations (
				version    INTEGER PRIMARY KEY,
				name       TEXT NOT NULL,
				applied_at INTEGER NOT NULL
			);

			CREATE TABLE users (
				user_id    INTEGER PRIMARY KEY,
				username   TEXT NOT NULL UNIQUE COLLATE NOCASE,
				password   TEXT NOT NULL,
				role       TEXT NOT NULL DEFAULT 'user',
				joined     INTEGER NOT NULL,
				last_login INTEGER NOT NULL DEFAULT 0,
				email      TEXT NOT NULL DEFAULT '',
				data       BLOB NOT NULL
			);
			CREATE INDEX idx_users_role ON users(role);
			CREATE INDEX idx_users_last_login ON users(last_login);

			CREATE TABLE room_instances (
				room_id    INTEGER PRIMARY KEY,
				zone       TEXT NOT NULL,
				data       BLOB NOT NULL,
				updated_at INTEGER NOT NULL
			);
			CREATE INDEX idx_room_instances_zone ON room_instances(zone);
		`,
	},
}

// applyMigrations runs any pending migrations in order. It is idempotent:
// running it twice is safe.
//
// The first migration creates schema_migrations itself, so we have to
// detect the bootstrap case (no schema_migrations table yet) and treat
// that as "current version 0".
func applyMigrations(ctx context.Context, db *sql.DB) error {
	current, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read current schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

// currentSchemaVersion returns the highest applied migration version, or
// 0 if the schema_migrations table does not exist yet.
func currentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	// Check if schema_migrations exists. SQLite-specific query.
	var name string
	err := db.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&name)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var version sql.NullInt64
	err = db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// applyOne runs a single migration in a transaction and records it in
// schema_migrations. If the migration SQL creates schema_migrations
// itself (the first migration), we record the row after the CREATE
// inside the same transaction.
func applyOne(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("exec migration sql: %w", err)
	}

	// Record this migration. schema_migrations exists by now, either
	// because a previous migration created it, or because this migration
	// (version 1) just did.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit()
}
