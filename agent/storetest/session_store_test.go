package storetest_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/storetest"
)

func TestMemorySessionStoreContract(t *testing.T) {
	if err := storetest.TestSessionStore(t.Context(), storetest.NewMemorySessionStore()); err != nil {
		t.Fatal(err)
	}
}
