package bedrockkb

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/internal/conformance"
)

func TestStoreConformance(t *testing.T) {
	conformance.Run(t, new(Store), conformance.Capabilities{
		Indexer: false, Searcher: true, IDDeleter: false, FilterDeleter: false,
	})
}
