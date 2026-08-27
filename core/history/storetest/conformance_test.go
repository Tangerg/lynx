package storetest_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/history/inmemory"
	"github.com/Tangerg/lynx/core/history/storetest"
)

func TestRun(t *testing.T) {
	storetest.Run(t, new(inmemory.Store), storetest.Capabilities{
		Reader: true, Writer: true, Clearer: true,
		Lister: true, Replacer: true, Counter: true,
	})
}
