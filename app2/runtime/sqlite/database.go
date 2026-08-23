package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const SchemaEpoch = 16

var (
	ErrInvalidConfig  = errors.New("sqlite: invalid config")
	ErrSchemaMismatch = errors.New("sqlite: schema epoch mismatch")
)

type Config struct {
	Path             string
	CreatedByVersion string
}

type Metadata struct {
	SchemaEpoch          int
	StoreID              string
	IdempotencyNamespace string
	CreatedByVersion     string
}

type Database struct {
	database *sql.DB
	metadata Metadata

	closeOnce sync.Once
	closeErr  error
}

func Open(ctx context.Context, config Config) (_ *Database, err error) {
	if !filepath.IsAbs(config.Path) {
		return nil, fmt.Errorf("%w: path must be absolute", ErrInvalidConfig)
	}
	if config.CreatedByVersion == "" {
		return nil, fmt.Errorf("%w: created-by version is required", ErrInvalidConfig)
	}
	path := filepath.Clean(config.Path)
	if err := preparePath(path); err != nil {
		return nil, err
	}

	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	defer func() {
		if err != nil {
			err = errors.Join(err, database.Close())
		}
	}()

	if err := configure(ctx, database); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("sqlite: protect database: %w", err)
	}
	metadata, err := openMetadata(ctx, database, config.CreatedByVersion)
	if err != nil {
		return nil, err
	}
	if err := createSchema(ctx, database); err != nil {
		return nil, err
	}
	instance := &Database{database: database, metadata: metadata}
	if err := instance.Ready(ctx); err != nil {
		return nil, err
	}
	return instance, nil
}

func (database *Database) Metadata() Metadata { return database.metadata }

func (database *Database) Ready(ctx context.Context) error {
	if database == nil || database.database == nil {
		return errors.New("sqlite: database is not open")
	}
	var result string
	if err := database.database.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("sqlite: quick check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("sqlite: quick check reported %q", result)
	}
	return nil
}

func (database *Database) Close() error {
	if database == nil || database.database == nil {
		return nil
	}
	database.closeOnce.Do(func() { database.closeErr = database.database.Close() })
	return database.closeErr
}

func preparePath(path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("sqlite: create data directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("sqlite: inspect data directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: data directory must be private", ErrInvalidConfig)
	}
	fileInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect database: %w", err)
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: database path must be a private regular file", ErrInvalidConfig)
	}
	return nil
}

func configure(ctx context.Context, database *sql.DB) error {
	statements := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("sqlite: configure %q: %w", statement, err)
		}
	}
	return nil
}

func openMetadata(ctx context.Context, database *sql.DB, version string) (Metadata, error) {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return Metadata{}, fmt.Errorf("sqlite: begin metadata transaction: %w", err)
	}
	defer transaction.Rollback()

	if _, err := transaction.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS runtime_metadata (
			singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
			schema_epoch INTEGER NOT NULL,
			store_id TEXT NOT NULL UNIQUE,
			idempotency_namespace TEXT NOT NULL UNIQUE,
			created_by_version TEXT NOT NULL
		)`); err != nil {
		return Metadata{}, fmt.Errorf("sqlite: create metadata table: %w", err)
	}
	storeID, err := newID("sto_")
	if err != nil {
		return Metadata{}, err
	}
	namespace, err := newID("idp_")
	if err != nil {
		return Metadata{}, err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT OR IGNORE INTO runtime_metadata (
			singleton, schema_epoch, store_id, idempotency_namespace, created_by_version
		) VALUES (1, ?, ?, ?, ?)`, SchemaEpoch, storeID, namespace, version); err != nil {
		return Metadata{}, fmt.Errorf("sqlite: initialize metadata: %w", err)
	}

	var metadata Metadata
	if err := transaction.QueryRowContext(ctx, `
		SELECT schema_epoch, store_id, idempotency_namespace, created_by_version
		FROM runtime_metadata WHERE singleton = 1`).Scan(
		&metadata.SchemaEpoch,
		&metadata.StoreID,
		&metadata.IdempotencyNamespace,
		&metadata.CreatedByVersion,
	); err != nil {
		return Metadata{}, fmt.Errorf("sqlite: read metadata: %w", err)
	}
	if metadata.SchemaEpoch != SchemaEpoch {
		return Metadata{}, fmt.Errorf("%w: database has epoch %d, build requires %d", ErrSchemaMismatch, metadata.SchemaEpoch, SchemaEpoch)
	}
	if metadata.StoreID == "" || metadata.IdempotencyNamespace == "" || metadata.CreatedByVersion == "" {
		return Metadata{}, errors.New("sqlite: metadata is incomplete")
	}
	if err := transaction.Commit(); err != nil {
		return Metadata{}, fmt.Errorf("sqlite: commit metadata: %w", err)
	}
	return metadata, nil
}

func newID(prefix string) (string, error) {
	encoded := make([]byte, 18)
	if _, err := rand.Read(encoded); err != nil {
		return "", fmt.Errorf("sqlite: create identity: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}
