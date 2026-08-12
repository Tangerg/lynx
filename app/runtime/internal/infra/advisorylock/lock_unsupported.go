//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package advisorylock

import (
	"context"
	"os"
)

func tryFile(*os.File) (*Lease, error) { return nil, ErrUnsupported }

func tryDirectory(string) (*Lease, error) { return nil, ErrUnsupported }

func acquireDirectory(context.Context, string) (*Lease, error) { return nil, ErrUnsupported }
