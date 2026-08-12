package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Availability string

const (
	Available Availability = "available"
	Missing   Availability = "missing"
)

func (availability Availability) Valid() bool {
	return availability == Available || availability == Missing
}

type Workspace struct {
	Path         string
	ProjectRoot  string
	Availability Availability
}

func (workspace Workspace) Validate() error {
	switch {
	case strings.TrimSpace(workspace.Path) == "":
		return errors.New("workspace path is empty")
	case !filepath.IsAbs(workspace.Path):
		return errors.New("workspace path is not absolute")
	case strings.TrimSpace(workspace.ProjectRoot) == "":
		return errors.New("workspace project root is empty")
	case !filepath.IsAbs(workspace.ProjectRoot):
		return errors.New("workspace project root is not absolute")
	case !workspace.Availability.Valid():
		return fmt.Errorf("workspace availability %q is invalid", workspace.Availability)
	default:
		return nil
	}
}

func (workspace Workspace) IsAvailable() bool { return workspace.Availability == Available }

type Summary struct {
	Workspace  Workspace
	Name       string
	Sessions   int
	LastActive *time.Time
}

func (summary Summary) Clone() Summary {
	if summary.LastActive != nil {
		summary.LastActive = new(*summary.LastActive)
	}
	return summary
}

func (summary Summary) Validate() error {
	if err := summary.Workspace.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(summary.Name) == "" {
		return errors.New("workspace summary name is empty")
	}
	if summary.Sessions < 0 {
		return errors.New("workspace session count is negative")
	}
	return nil
}

type ResolveRequest struct {
	Path string
}

func (request ResolveRequest) Validate() error {
	if request.Path != "" && !filepath.IsAbs(request.Path) {
		return errors.New("workspace resolve path is not absolute")
	}
	return nil
}
