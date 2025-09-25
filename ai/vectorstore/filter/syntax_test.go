package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter"
)

func TestEQ(t *testing.T) {
	ge := filter.GE("name", 18)
	t.Log(ge.Left, ge.Op, ge.Right)
}
