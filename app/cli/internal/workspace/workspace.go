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

func (a Availability) Valid() bool {
	return a == Available || a == Missing
}

type Workspace struct {
	Path         string
	ProjectRoot  string
	Availability Availability
}

func (w Workspace) Validate() error {
	switch {
	case strings.TrimSpace(w.Path) == "":
		return errors.New("workspace path is empty")
	case !filepath.IsAbs(w.Path):
		return errors.New("workspace path is not absolute")
	case strings.TrimSpace(w.ProjectRoot) == "":
		return errors.New("workspace project root is empty")
	case !filepath.IsAbs(w.ProjectRoot):
		return errors.New("workspace project root is not absolute")
	case !w.Availability.Valid():
		return fmt.Errorf("workspace availability %q is invalid", w.Availability)
	default:
		return nil
	}
}

func (w Workspace) IsAvailable() bool { return w.Availability == Available }

type Summary struct {
	Workspace  Workspace
	Name       string
	Sessions   int
	LastActive *time.Time
}

func (s Summary) Clone() Summary {
	if s.LastActive != nil {
		s.LastActive = new(*s.LastActive)
	}
	return s
}

func (s Summary) Validate() error {
	if err := s.Workspace.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("workspace summary name is empty")
	}
	if s.Sessions < 0 {
		return errors.New("workspace session count is negative")
	}
	return nil
}

type ResolveRequest struct {
	Path string
}

func (r ResolveRequest) Validate() error {
	if r.Path != "" && !filepath.IsAbs(r.Path) {
		return errors.New("workspace resolve path is not absolute")
	}
	return nil
}
