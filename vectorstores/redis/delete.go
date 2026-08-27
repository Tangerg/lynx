package redis

import (
	"context"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// Delete looks up documents matching the filter via FT.SEARCH, then
// removes the underlying keys with DEL.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("redis.Store.DeleteWhere: %w", err)
	}

	var query string
	query, err = s.buildFilterQuery(expr)
	if err != nil {
		return err
	}
	if query == "*" {
		return errors.New("redis: refusing to DELETE on empty filter — pass a non-trivial expression")
	}

	const pageSize = 500
	opts := &goredis.FTSearchOptions{
		NoContent:      true,
		LimitOffset:    0,
		Limit:          pageSize,
		DialectVersion: 2,
	}
	for {
		result, err := s.client.FTSearchWithArgs(ctx, s.indexName, query, opts).Result()
		if err != nil {
			return fmt.Errorf("redis: FT.SEARCH %s: %w", s.indexName, err)
		}
		if len(result.Docs) == 0 {
			return nil
		}
		keys := make([]string, 0, len(result.Docs))
		for _, hit := range result.Docs {
			keys = append(keys, hit.ID)
		}
		if _, err = s.client.Del(ctx, keys...).Result(); err != nil {
			return fmt.Errorf("redis: DEL: %w", err)
		}
		if len(result.Docs) < pageSize {
			return nil
		}
	}
}

// DeleteIDs removes documents by id, resolving each to its HASH key
// `<KeyPrefix><id>` and issuing a single DEL. An empty slice is a
// no-op; unknown ids are silently ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = s.keyPrefix + id
	}
	if _, err = s.client.Del(ctx, keys...).Result(); err != nil {
		return fmt.Errorf("redis: DEL: %w", err)
	}
	return nil
}
