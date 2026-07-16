package neo4j_test

import (
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Tangerg/lynx/chathistory"
	neo4jstore "github.com/Tangerg/lynx/chathistory/neo4j"
)

// stubDriver is a non-nil but never-used DriverWithContext for tests
// that only exercise validation. The driver is never queried.
func stubDriver() neo4j.DriverWithContext {
	drv, _ := neo4j.NewDriverWithContext("bolt://127.0.0.1:0", neo4j.NoAuth())
	return drv
}

func TestNewRequiresDriver(t *testing.T) {
	_, err := neo4jstore.New(neo4jstore.Config{})
	if err == nil {
		t.Fatal("expected error when Driver is nil")
	}
	if !strings.Contains(err.Error(), "Driver") {
		t.Fatalf("err = %v; should mention Driver", err)
	}
}

func TestNewRejectsBadLabel(t *testing.T) {
	_, err := neo4jstore.New(neo4jstore.Config{
		Driver: stubDriver(),
		Label:  "Bad-Label",
	})
	if err == nil {
		t.Fatal("expected error on label with hyphen")
	}
}

func TestNewAcceptsDefaults(t *testing.T) {
	_, err := neo4jstore.New(neo4jstore.Config{Driver: stubDriver()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*neo4jstore.Store)(nil)
}
