package protocol

import "testing"

func TestPublishedSnapshotsAreCallerOwned(t *testing.T) {
	topics := RuntimeTopics()
	topics[0] = "rewritten"
	if RuntimeTopics()[0] != TopicFilesChanged {
		t.Fatal("mutating one RuntimeTopics result rewrote the protocol vocabulary")
	}

	samples := CanonicalSamples()
	original := samples[0]
	samples[0] = CanonicalSample{}
	if got := CanonicalSamples()[0]; got != original {
		t.Fatal("mutating one CanonicalSamples result rewrote the artifact catalog")
	}
}
