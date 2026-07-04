package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
)

func (s *Server) claimWorkingTreeRun(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return s.workingTrees.ClaimRun(fspath.Canonical(cwd))
}

func (s *Server) claimWorkingTreeMutation(cwd string) (lifecycle.WorkingTreeAdmission, bool) {
	return s.workingTrees.ClaimMutation(fspath.Canonical(cwd))
}
