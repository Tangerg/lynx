package typesense

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestStoreConformance(t *testing.T) {
	storetest.Run(t, new(Store), storetest.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: false, FilterDeleter: true,
	})
}
