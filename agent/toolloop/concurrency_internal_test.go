package toolloop

import "testing"

func TestSegmentEndHonorsExclusiveCallsAndResourceConflicts(t *testing.T) {
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
			name: "independent calls share segment",
			plans: []callPlan{
				{concurrent: true},
				{concurrent: true, key: "a"},
				{concurrent: true, key: "b"},
			},
			want: 3,
		},
		{
			name: "duplicate key starts next segment",
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
			name:  "later segment",
			plans: []callPlan{{}, {concurrent: true}, {concurrent: true}},
			start: 1,
			want:  3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := segmentEnd(test.plans, test.start); got != test.want {
				t.Fatalf("segmentEnd() = %d, want %d", got, test.want)
			}
		})
	}
}
