package toolloop

import "testing"

func TestBatchEndHonorsExclusiveCallsAndResourceConflicts(t *testing.T) {
	tests := []struct {
		name  string
		plans []callPlan
		start int
		want  int
	}{
		{
			name:  "exclusive call stands alone",
			plans: []callPlan{{}, {concurrent: true}},
			want:  1,
		},
		{
			name: "independent calls share batch",
			plans: []callPlan{
				{concurrent: true},
				{concurrent: true, key: "a"},
				{concurrent: true, key: "b"},
			},
			want: 3,
		},
		{
			name: "duplicate key starts next batch",
			plans: []callPlan{
				{concurrent: true, key: "same"},
				{concurrent: true, key: "other"},
				{concurrent: true, key: "same"},
			},
			want: 2,
		},
		{
			name: "exclusive call ends concurrent prefix",
			plans: []callPlan{
				{concurrent: true},
				{},
				{concurrent: true},
			},
			want: 1,
		},
		{
			name:  "later batch",
			plans: []callPlan{{}, {concurrent: true}, {concurrent: true}},
			start: 1,
			want:  3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := batchEnd(test.plans, test.start); got != test.want {
				t.Fatalf("batchEnd() = %d, want %d", got, test.want)
			}
		})
	}
}
