package s3vectors

import (
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/conformance"
)

func TestStoreConformance(t *testing.T) {
	conformance.Run(t, new(Store), conformance.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: false, FilterDeleter: true,
	})
}
