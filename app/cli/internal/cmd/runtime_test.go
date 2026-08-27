package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
	"github.com/Tangerg/scope/app/cli/internal/backend"
	"github.com/Tangerg/scope/app/cli/internal/runtimeprofile"
)

func TestRuntimeInfoWritesCompleteHumanAndMachineProfiles(t *testing.T) {
	t.Parallel()

	profile := commandRuntimeProfile()
	for _, test := range []struct {
		name  string
		args  []string
		check func(*testing.T, string)
	}{
		{
			name: "human", args: []string{"runtime", "info"},
			check: func(t *testing.T, output string) {
				for _, want := range []string{
					"lyra-runtime 1.2.3", "protocol", "/workspace", "segment.started", "files.changed",
					"feature mcp", "client opt-in requested", "available", "1024 events", "600 seconds", "32 watches",
				} {
					if !strings.Contains(output, want) {
						t.Fatalf("runtime info omitted %q:\n%s", want, output)
					}
				}
			},
		},
		{
			name: "JSON", args: []string{"runtime", "info", "--json"},
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, `"maxConcurrentRuns":`) {
					t.Fatalf("runtime profile JSON omitted the optional concurrency limit: %s", output)
				}
				var decoded runtimeprofile.Profile
				if err := json.Unmarshal([]byte(output), &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded.Server != profile.Server || len(decoded.Features) != len(profile.Features) ||
					decoded.Limits.IdempotencyRetentionSeconds != 600 || decoded.Limits.RunReplay.MaxBytes != 1<<20 {
					t.Fatalf("runtime profile JSON = %+v", decoded)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			root := NewRoot(Dependencies{OpenRuntime: func(context.Context) (backend.Services, error) {
				return backend.Services{Agent: mock.New(), RuntimeProfile: new(profile.Clone())}, nil
			}})
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(test.args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			test.check(t, output.String())
		})
	}
}

func commandRuntimeProfile() runtimeprofile.Profile {
	return runtimeprofile.Profile{
		Protocol:  runtimeprofile.Protocol{Version: "2.0"},
		Server:    runtimeprofile.Server{Name: "lyra-runtime", Version: "1.2.3", DefaultWorkspace: "/workspace", Home: "/home/test"},
		RunEvents: []string{"segment.started"}, RuntimeTopics: []string{"files.changed"},
		StreamingMethods: []string{"runs.start"},
		Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			"mcp": {
				Enabled: true, ClientOptIn: true, ClientRequested: true, RequiredByRunProtocol: true,
			},
		},
		Limits: runtimeprofile.Limits{
			MaxConcurrentRuns: 4, IdempotencyRetentionSeconds: 600, IdempotencyNamespace: "idp_test",
			RunReplay:                        runtimeprofile.ReplayLimits{Scope: "runtimeInstanceRootSegment", MaxEvents: 1024, MaxBytes: 1 << 20},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              runtimeprofile.SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
		},
	}
}
