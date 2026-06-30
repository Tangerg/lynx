package hooks

import (
	"context"
	"strings"
	"testing"
	"time"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestShell_CommandReceivesStdin(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), domainhooks.CommandRequest{
		Command: `grep -q '"event":"UserPromptSubmit"' && echo '{"injectContext":"saw-event"}'`,
		Stdin:   []byte(`{"event":"UserPromptSubmit"}`),
		Timeout: time.Second,
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if strings.TrimSpace(string(got.Stdout)) != `{"injectContext":"saw-event"}` {
		t.Fatalf("stdout = %q", got.Stdout)
	}
}

func TestShell_Timeout(t *testing.T) {
	got := Shell{}.RunHookCommand(context.Background(), domainhooks.CommandRequest{
		Command: `sleep 5`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
}
