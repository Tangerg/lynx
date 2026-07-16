package core_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"
)

func TestAllowsPermissions(t *testing.T) {
	cases := []struct {
		name     string
		allowed  []core.ToolGroupPermission
		required []core.ToolGroupPermission
		want     bool
	}{
		{
			name:     "empty requirement is always allowed",
			required: nil,
			want:     true,
		},
		{
			name:     "exact match satisfies",
			allowed:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     true,
		},
		{
			name:     "allowed superset satisfies",
			allowed:  []core.ToolGroupPermission{core.ToolGroupHostAccess, core.ToolGroupInternetAccess},
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     true,
		},
		{
			name:     "required superset is rejected",
			allowed:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess, core.ToolGroupInternetAccess},
			want:     false,
		},
		{
			name:     "missing required permission is rejected",
			allowed:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			required: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
			want:     false,
		},
		{
			name:     "empty allowance rejects any required permission",
			allowed:  nil,
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.AllowsPermissions(tc.allowed, tc.required); got != tc.want {
				t.Errorf("AllowsPermissions(%v, %v) = %v, want %v",
					tc.allowed, tc.required, got, tc.want)
			}
		})
	}
}

func TestLazyToolGroupResolver(t *testing.T) {
	loads := 0
	tool, err := tools.New[struct{}, string](
		tools.Config{Name: "dynamic"},
		func(context.Context, struct{}) (string, error) { return "ok", nil },
	)
	if err != nil {
		t.Fatalf("tools.New: %v", err)
	}
	info := core.ToolGroupInfo{Role: "research"}
	resolver, err := core.NewLazyToolGroupResolver("remote-research", info, func(context.Context) ([]tools.Tool, error) {
		loads++
		return []tools.Tool{tool}, nil
	})
	if err != nil {
		t.Fatalf("NewLazyToolGroupResolver: %v", err)
	}
	if resolver.Name() != "remote-research" {
		t.Fatalf("Name = %q", resolver.Name())
	}
	if group, ok, err := resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: "other"}); err != nil || ok || group != nil {
		t.Fatalf("miss = %#v, %v, %v", group, ok, err)
	}
	group, ok, err := resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: "research"})
	if err != nil || !ok || group == nil {
		t.Fatalf("Resolve = %#v, %v, %v", group, ok, err)
	}
	for range 2 {
		got, loadErr := group.Tools(t.Context())
		if loadErr != nil || len(got) != 1 {
			t.Fatalf("Tools = %v, %v", got, loadErr)
		}
	}
	if loads != 1 {
		t.Fatalf("loader calls = %d, want 1", loads)
	}
}

func TestLazyToolGroupResolverRejectsInvalidConfig(t *testing.T) {
	info := core.ToolGroupInfo{Role: "role"}
	loader := func(context.Context) ([]tools.Tool, error) { return nil, nil }
	cases := []struct {
		name     string
		resolver func() error
	}{
		{"empty name", func() error {
			_, err := core.NewLazyToolGroupResolver("", info, loader)
			return err
		}},
		{"empty role", func() error {
			_, err := core.NewLazyToolGroupResolver("name", core.ToolGroupInfo{}, loader)
			return err
		}},
		{"nil loader", func() error {
			_, err := core.NewLazyToolGroupResolver("name", info, nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.resolver(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestToolGroupPermissionString(t *testing.T) {
	cases := []struct {
		p    core.ToolGroupPermission
		want string
	}{
		{core.ToolGroupHostAccess, "host_access"},
		{core.ToolGroupInternetAccess, "internet_access"},
		{core.ToolGroupPermission(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.p.String(); got != tc.want {
			t.Errorf("(%d).String() = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestLazyToolGroupCopiesInfo(t *testing.T) {
	info := core.ToolGroupInfo{
		Role:        "web",
		Permissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	}
	group := core.NewLazyToolGroup(info, nil)
	info.Permissions[0] = core.ToolGroupHostAccess
	got := group.Info()
	if len(got.Permissions) != 1 || got.Permissions[0] != core.ToolGroupInternetAccess {
		t.Fatalf("Info().Permissions = %v, want [internet_access]", got.Permissions)
	}
	got.Permissions[0] = core.ToolGroupHostAccess
	if again := group.Info(); again.Permissions[0] != core.ToolGroupInternetAccess {
		t.Errorf("Info returned mutable permissions: %v", again.Permissions)
	}
}
