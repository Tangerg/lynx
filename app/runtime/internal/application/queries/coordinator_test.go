package queries

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type fakeTranscript struct {
	items []transcript.SequencedItem
	runs  []transcript.Run

	session       string
	afterSequence int64
	limit         int
}

func (f *fakeTranscript) PageItems(_ context.Context, sessionID string, afterSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.session = sessionID
	f.afterSequence = afterSequence
	f.limit = limit
	var out []transcript.SequencedItem
	for _, entry := range f.items {
		if entry.Sequence <= afterSequence {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, entry)
	}
	return out, nil
}

func (f *fakeTranscript) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	f.session = sessionID
	return f.runs, nil
}

type fakeInterrupts struct {
	pending []interrupts.Pending
	session string
}

func (f *fakeInterrupts) ListPage(_ context.Context, sessionID string, _ int64, _ string, _ int) ([]interrupts.Pending, error) {
	f.session = sessionID
	return f.pending, nil
}

func sequencedItems(count int) []transcript.SequencedItem {
	out := make([]transcript.SequencedItem, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, transcript.SequencedItem{
			Sequence: int64(i),
			Item:     transcript.Item{ID: "it_" + strconv.Itoa(i)},
		})
	}
	return out
}

func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(1), runs: []transcript.Run{{ID: "run_1"}}}
	ints := &fakeInterrupts{pending: []interrupts.Pending{{RunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, Interrupts: ints})

	page, err := c.ListItemPage(ctx, "ses_1", "", 0)
	if err != nil || len(page.Items) != 1 || len(page.Runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListItemPage items=%d runs=%d session=%q err=%v", len(page.Items), len(page.Runs), tx.session, err)
	}

	pending, err := c.ListPendingInterruptPage(ctx, "ses_2", "", 0)
	if err != nil || len(pending.Rows) != 1 || ints.session != "ses_2" {
		t.Fatalf("ListPendingInterruptPage pending=%d session=%q err=%v", len(pending.Rows), ints.session, err)
	}
}

// The page is cut by the query, not after the fact: the read asks for exactly one
// row more than it will return, which is both how "there is more" is known and
// what keeps a long session's history out of memory.
func TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := New(Dependencies{Transcript: tx})

	first, err := c.ListItemPage(ctx, "ses_1", "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if tx.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", tx.limit)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "it_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two items and a cursor", first)
	}

	second, err := c.ListItemPage(ctx, "ses_1", first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 2 {
		t.Fatalf("second page sought past %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID != "it_3" {
		t.Fatalf("second page = %+v, want it_3 onward", second)
	}

	last, err := c.ListItemPage(ctx, "ses_1", second.NextCursor, 2)
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	if len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatalf("last page = %+v, want the tail and no cursor", last)
	}
}

// A cursor from another session would page this one against positions it never
// enumerated. Restarting from the top instead of refusing would hand the client
// rows it had already read, as if they were new.
func TestListItemPageRefusesAForeignCursor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := New(Dependencies{Transcript: tx})

	other, err := c.ListItemPage(ctx, "ses_other", "", 2)
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, "ses_1", other.NextCursor, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListItemPage(ctx, "ses_1", "not-a-cursor", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}

func TestListItemPageRejectsANegativeLimit(t *testing.T) {
	c := New(Dependencies{Transcript: &fakeTranscript{items: sequencedItems(1)}})
	if _, err := c.ListItemPage(context.Background(), "ses_1", "", -1); err == nil {
		t.Fatal("negative limit returned no error")
	}
}
