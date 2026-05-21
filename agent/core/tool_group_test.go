package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestPermissionsSatisfy(t *testing.T) {
	cases := []struct {
		name     string
		required []core.ToolGroupPermission
		granted  []core.ToolGroupPermission
		want     bool
	}{
		{
			name:    "empty granted is always allowed",
			granted: nil,
			want:    true,
		},
		{
			name:     "exact match satisfies",
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			granted:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     true,
		},
		{
			name:     "subset granted satisfies",
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess, core.ToolGroupInternetAccess},
			granted:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     true,
		},
		{
			name:     "superset granted is rejected",
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			granted:  []core.ToolGroupPermission{core.ToolGroupHostAccess, core.ToolGroupInternetAccess},
			want:     false,
		},
		{
			name:     "missing required permission is rejected",
			required: []core.ToolGroupPermission{core.ToolGroupHostAccess},
			granted:  []core.ToolGroupPermission{core.ToolGroupInternetAccess},
			want:     false,
		},
		{
			name:     "empty requirement rejects any granted permission",
			required: nil,
			granted:  []core.ToolGroupPermission{core.ToolGroupHostAccess},
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.PermissionsSatisfy(tc.required, tc.granted); got != tc.want {
				t.Errorf("PermissionsSatisfy(%v, %v) = %v, want %v",
					tc.required, tc.granted, got, tc.want)
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

func TestSimpleToolGroupMetadata_Permissions(t *testing.T) {
	m := core.SimpleToolGroupMetadata{
		RoleText:           "web",
		PermissionsGranted: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	}
	got := m.Permissions()
	if len(got) != 1 || got[0] != core.ToolGroupInternetAccess {
		t.Errorf("Permissions() = %v, want [internet_access]", got)
	}

	empty := core.SimpleToolGroupMetadata{RoleText: "noop"}
	if got := empty.Permissions(); len(got) != 0 {
		t.Errorf("empty metadata Permissions() = %v, want []", got)
	}
}
