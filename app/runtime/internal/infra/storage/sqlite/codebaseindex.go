package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// CodebaseIndexStore persists the @codebase semantic index against SQLite:
// per-cwd meta (codebase_index), per-file content hashes (codebase_files), and
// the chunk text + embedding vectors (codebase_chunks, vectors as little-endian
// float32 BLOBs). The DB must have been opened via [Open].
type CodebaseIndexStore struct {
	db *sql.DB
}

var _ codebaseindex.Store = (*CodebaseIndexStore)(nil)

// NewCodebaseIndexStore wires the given *sql.DB to the codebaseindex.Store surface.
func NewCodebaseIndexStore(db *sql.DB) *CodebaseIndexStore {
	return &CodebaseIndexStore{db: db}
}

func (s *CodebaseIndexStore) Meta(ctx context.Context, cwd string) (codebaseindex.Meta, bool, error) {
	var (
		m         = codebaseindex.Meta{Cwd: cwd}
		indexedMs int64
		truncated int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT model_id, indexed_at, file_count, chunk_count, truncated
		 FROM codebase_index WHERE cwd = ?`, cwd).
		Scan(&m.ModelID, &indexedMs, &m.FileCount, &m.ChunkCount, &truncated)
	if errors.Is(err, sql.ErrNoRows) {
		return codebaseindex.Meta{}, false, nil
	}
	if err != nil {
		return codebaseindex.Meta{}, false, fmt.Errorf("sqlite: codebase meta: %w", err)
	}
	m.IndexedAt = fromMillis(indexedMs)
	m.Truncated = truncated != 0
	return m, true, nil
}

func (s *CodebaseIndexStore) SetMeta(ctx context.Context, m codebaseindex.Meta) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO codebase_index (cwd, model_id, indexed_at, file_count, chunk_count, truncated)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(cwd) DO UPDATE SET
		   model_id = excluded.model_id, indexed_at = excluded.indexed_at,
		   file_count = excluded.file_count, chunk_count = excluded.chunk_count,
		   truncated = excluded.truncated`,
		m.Cwd, m.ModelID, toMillis(m.IndexedAt), m.FileCount, m.ChunkCount, boolToInt(m.Truncated))
	if err != nil {
		return fmt.Errorf("sqlite: set codebase meta: %w", err)
	}
	return nil
}

func (s *CodebaseIndexStore) FileHashes(ctx context.Context, cwd string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, hash FROM codebase_files WHERE cwd = ?`, cwd)
	if err != nil {
		return nil, fmt.Errorf("sqlite: codebase file hashes: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, fmt.Errorf("sqlite: scan codebase file: %w", err)
		}
		out[path] = hash
	}
	return out, rows.Err()
}

func (s *CodebaseIndexStore) ReplaceFile(ctx context.Context, cwd, path, hash string, chunks []codebaseindex.Chunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: codebase replace begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM codebase_chunks WHERE cwd = ? AND path = ?`, cwd, path); err != nil {
		return fmt.Errorf("sqlite: codebase clear file chunks: %w", err)
	}
	for _, c := range chunks {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO codebase_chunks (cwd, path, start_line, end_line, text, embedding)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			cwd, path, c.StartLine, c.EndLine, c.Text, encodeVec(c.Embedding)); err != nil {
			return fmt.Errorf("sqlite: codebase insert chunk: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO codebase_files (cwd, path, hash) VALUES (?, ?, ?)
		 ON CONFLICT(cwd, path) DO UPDATE SET hash = excluded.hash`,
		cwd, path, hash); err != nil {
		return fmt.Errorf("sqlite: codebase upsert file hash: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: codebase replace commit: %w", err)
	}
	return nil
}

func (s *CodebaseIndexStore) DeleteFile(ctx context.Context, cwd, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: codebase delete begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM codebase_chunks WHERE cwd = ? AND path = ?`, cwd, path); err != nil {
		return fmt.Errorf("sqlite: codebase delete chunks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM codebase_files WHERE cwd = ? AND path = ?`, cwd, path); err != nil {
		return fmt.Errorf("sqlite: codebase delete file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: codebase delete commit: %w", err)
	}
	return nil
}

func (s *CodebaseIndexStore) AllChunks(ctx context.Context, cwd string) ([]codebaseindex.Chunk, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, start_line, end_line, text, embedding FROM codebase_chunks WHERE cwd = ?`, cwd)
	if err != nil {
		return nil, fmt.Errorf("sqlite: codebase chunks: %w", err)
	}
	defer rows.Close()
	var out []codebaseindex.Chunk
	for rows.Next() {
		var (
			c    codebaseindex.Chunk
			blob []byte
		)
		if err := rows.Scan(&c.Path, &c.StartLine, &c.EndLine, &c.Text, &blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan codebase chunk: %w", err)
		}
		c.Embedding = decodeVec(blob)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *CodebaseIndexStore) Clear(ctx context.Context, cwd string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: codebase clear begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"codebase_chunks", "codebase_files", "codebase_index"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE cwd = ?`, cwd); err != nil {
			return fmt.Errorf("sqlite: codebase clear %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: codebase clear commit: %w", err)
	}
	return nil
}

// encodeVec serializes an embedding as little-endian float32 (4 bytes each).
func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

// decodeVec is encodeVec's inverse.
func decodeVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
