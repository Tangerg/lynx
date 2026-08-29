package agenttest_test

import (
	"testing"

	"github.com/Tangerg/scope/agent/agenttest"
)

func TestMemoryTreeDurabilityConformance(t *testing.T) {
	agenttest.RunTreeDurabilityConformance(t, func() agenttest.TreeDurabilityConformanceDriver {
		return agenttest.NewMemoryTreeDurability()
	})
}
