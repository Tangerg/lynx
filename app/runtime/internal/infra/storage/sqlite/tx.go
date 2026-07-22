package sqlite

import (
	"context"
	"database/sql"
)

// Cross-store transactions. The stores in this package each own their own
// table(s) but share one *sql.DB, and a few delivery-layer operations
// (sessions.import, sessions.rollback) need a write-set that spans several of
// them to be atomic — a mid-sequence failure must leave no partial state.
//
// A caller-supplied transaction is carried on the context: [RunInTx] begins it
// (or joins an outer one) and stashes it under txKey; every store statement
// runs through [conn], which uses that transaction when present and the shared
// pool otherwise. This is mandatory rather than an optimization: the pool runs
// at MaxOpenConns(1), so a store that opened its OWN BeginTx while a context
// transaction was live would deadlock — the single connection is already held.

// execer is the statement surface shared by *sql.DB and *sql.Tx, so a store
// method can run on either the pool or a caller's transaction unchanged.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// scanRow is the row surface shared by *sql.Row (QueryRow) and *sql.Rows (Query
// iteration), so one scan helper covers both a single Get and a List loop. Used
// by the goal / interrupt / agent-memory decoders.
type scanRow interface {
	Scan(dest ...any) error
}

type txKey struct{}

// conn returns the transaction carried on ctx by [RunInTx] if one is live,
// else the shared pool db. A store method MUST route through this (not the bare
// s.db) for any statement that should join a caller's cross-store transaction.
func conn(ctx context.Context, db *sql.DB) execer {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

// RunInTx runs fn inside a single transaction exposed to fn's store calls via
// the returned context (they pick it up through [conn]). It commits on success
// and rolls back on error or panic, so a multi-step write-set is atomic. A
// nested RunInTx joins the outer transaction (a single connection can't nest
// transactions, and the outer owns the commit), so a store method that wraps
// its own body in RunInTx is atomic standalone yet folds into a caller's
// transaction transparently.
func RunInTx(ctx context.Context, db *sql.DB, fn func(context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx) // already in a transaction — join it
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-raise after releasing the connection
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit()
}
