package elasticsearch

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/internal/conformance"
)

func TestStoreConformance(t *testing.T) {
	conformance.Run(t, new(Store), conformance.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: true, FilterDeleter: true,
	})
}
