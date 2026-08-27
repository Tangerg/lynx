package redis

import (
	"testing"

	"github.com/Tangerg/lynx/core/history/storetest"
)

func TestStoreConformance(t *testing.T) {
	storetest.Run(t, new(Store), storetest.Capabilities{
		Reader: true, Writer: true, Clearer: true, Lister: true,
	})
}
