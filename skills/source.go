package skills

import (
	"context"
	"fmt"
	"io/fs"
	"reflect"
)

// Source is the read-only repository that lists and loads skills. Its two
// operations mirror the first progressive-disclosure levels, so a consumer
// pulls in only as much as a task needs:
//
//   - List — name + description for every skill (level 1)
//   - Load — one skill's full instructions (level 2)
//
// Implementations must return valid Summary and Skill models, honor ctx
// cancellation, and return an error matching context.Canceled or
// context.DeadlineExceeded.
type Source interface {
	List(ctx context.Context) ([]Summary, error)
	Load(ctx context.Context, name string) (*Skill, error)
}

// ResourceSource extends [Source] with progressive-disclosure level 3:
// opening a resource bundled under a skill directory.
type ResourceSource interface {
	Source
	OpenResource(ctx context.Context, name, resource string) (fs.File, error)
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("skills: %s: %w", operation, err)
	}
	return nil
}
