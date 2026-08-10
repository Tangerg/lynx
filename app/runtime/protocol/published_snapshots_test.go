package protocol

import "testing"

func TestPublishedSnapshotsAreCallerOwned(t *testing.T) {
	topics := RuntimeTopics()
	topics[0] = "rewritten"
	if RuntimeTopics()[0] != TopicFilesChanged {
		t.Fatal("mutating one RuntimeTopics result rewrote the protocol vocabulary")
	}

}
