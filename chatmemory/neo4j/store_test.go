package neo4j_test

import (
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	neo4jmem "github.com/Tangerg/lynx/chatmemory/neo4j"
	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"
)

// stubDriver is a non-nil but never-used DriverWithContext for tests
// that only exercise validation. The driver is never queried.
func stubDriver() neo4j.DriverWithContext {
	drv, _ := neo4j.NewDriverWithContext("bolt://127.0.0.1:0", neo4j.NoAuth())
	return drv
}

func TestStoreConfig_DriverRequired(t *testing.T) {
	_, err := neo4jmem.NewStore(neo4jmem.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Driver is nil")
	}
	if !strings.Contains(err.Error(), "Driver") {
		t.Fatalf("err = %v; should mention Driver", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	if _, err := neo4jmem.NewStore(nil); err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStoreConfig_RejectsBadLabel(t *testing.T) {
	_, err := neo4jmem.NewStore(neo4jmem.StoreConfig{
		Driver: stubDriver(),
		Label:  "Bad-Label",
	})
	if err == nil {
		t.Fatal("expected error on label with hyphen")
	}
}

func TestStoreConfig_AcceptsDefaults(t *testing.T) {
	_, err := neo4jmem.NewStore(neo4jmem.StoreConfig{Driver: stubDriver()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_ImplementsMemoryStore(t *testing.T) {
	var _ chatmem.Store = (*neo4jmem.Store)(nil)
}
