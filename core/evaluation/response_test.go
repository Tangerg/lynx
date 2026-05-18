package evaluation

import "testing"

// TestParseScoredResponse_ExtractsFirstInRangeFloat ensures common LLM
// reply shapes all parse to the right score / Pass / Feedback split.
// Indirectly pins the regex + clamp behaviour at the function level so
// we catch parsing regressions without going through the LLM client.
func TestParseScoredResponse_ExtractsFirstInRangeFloat(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		thresh   float64
		score    float64
		pass     bool
		feedback string
	}{
		{
			name:     "bare float",
			input:    "0.85",
			thresh:   0.5,
			score:    0.85,
			pass:     true,
			feedback: "",
		},
		{
			name:     "score then reason on next line",
			input:    "0.7\nMostly correct but missed nuance.",
			thresh:   0.5,
			score:    0.7,
			pass:     true,
			feedback: "Mostly correct but missed nuance.",
		},
		{
			name:     "SCORE prefix",
			input:    "SCORE: 0.42 — partial overlap.",
			thresh:   0.5,
			score:    0.42,
			pass:     false,
			feedback: "— partial overlap.",
		},
		{
			name:     "leading number out of range, fallback to next",
			input:    "5 out of 10 = 0.5 — split decision.",
			thresh:   0.5,
			score:    0.5,
			pass:     true,
			feedback: "— split decision.",
		},
		{
			name:     "integer zero",
			input:    "0\nNot supported at all.",
			thresh:   0.5,
			score:    0.0,
			pass:     false,
			feedback: "Not supported at all.",
		},
		{
			name:     "integer one",
			input:    "1\nFully grounded.",
			thresh:   0.5,
			score:    1.0,
			pass:     true,
			feedback: "Fully grounded.",
		},
		{
			name:     "custom threshold flips verdict",
			input:    "0.6",
			thresh:   0.8,
			score:    0.6,
			pass:     false,
			feedback: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScoredResponse(tc.input, tc.thresh)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Score != tc.score {
				t.Errorf("Score = %v, want %v", got.Score, tc.score)
			}
			if got.Pass != tc.pass {
				t.Errorf("Pass = %v, want %v", got.Pass, tc.pass)
			}
			if got.Feedback != tc.feedback {
				t.Errorf("Feedback = %q, want %q", got.Feedback, tc.feedback)
			}
		})
	}
}

func TestParseScoredResponse_NoScoreErrors(t *testing.T) {
	cases := []string{
		"YES",
		"the answer is yes",
		"no numbers anywhere",
		"",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, err := parseScoredResponse(in, 0.5); err == nil {
				t.Fatalf("expected error for input %q", in)
			}
		})
	}
}

// TestParseScoredResponse_OutOfRangeNumbersSkipped covers replies that
// only contain numbers >1: "give 5/10" with no normalized score should
// fail rather than silently clamp.
func TestParseScoredResponse_OutOfRangeNumbersSkipped(t *testing.T) {
	if _, err := parseScoredResponse("5 out of 10", 0.5); err == nil {
		t.Fatal("expected error: no number in [0,1] present")
	}
}
