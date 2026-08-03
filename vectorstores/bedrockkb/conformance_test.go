package bedrockkb

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/storetest"
)

func TestStoreConformance(t *testing.T) {
	storetest.Run(t, new(Store), storetest.Capabilities{
		Indexer: false, Searcher: true, IDDeleter: false, FilterDeleter: false,
	})
}
