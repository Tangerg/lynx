package redis

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/conformance"
)

func TestStoreConformance(t *testing.T) {
	conformance.Run(t, new(Store), conformance.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: true, FilterDeleter: true,
	})
}
