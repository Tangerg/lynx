package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestToolGroupRequirementAllows(t *testing.T) {
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
			requirement := core.ToolGroupRequirement{AllowedPermissions: tc.allowed}
			if got := requirement.Allows(tc.required); got != tc.want {
				t.Errorf("ToolGroupRequirement.Allows(%v, %v) = %v, want %v",
					tc.allowed, tc.required, got, tc.want)
			}
		})
	}
}

func TestToolGroupContractsValidate(t *testing.T) {
	tests := []struct {
		name  string
		value interface{ Validate() error }
	}{
		{
			name:  "requirement needs role",
			value: core.ToolGroupRequirement{},
		},
		{
			name:  "requirement rejects unknown allowance",
			value: core.ToolGroupRequirement{Role: "research", AllowedPermissions: []core.ToolGroupPermission{99}},
		},
		{
			name:  "info rejects padded role",
			value: core.ToolGroupInfo{Role: " research "},
		},
		{
			name:  "info rejects unknown permission",
			value: core.ToolGroupInfo{Role: "research", Permissions: []core.ToolGroupPermission{99}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.value.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}

	valid := core.ToolGroupRequirement{
		Role:               "research",
		AllowedPermissions: []core.ToolGroupPermission{core.ToolGroupInternetAccess},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid requirement: %v", err)
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
