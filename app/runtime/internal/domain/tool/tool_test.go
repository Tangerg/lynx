package tool

import "testing"

func TestGroupVocabulary(t *testing.T) {
	for _, group := range []Group{GroupRoot, GroupDelegated} {
		if !group.Valid() {
			t.Errorf("Group %q is invalid", group)
		}
	}
	for _, group := range []Group{"", "role", "subtask"} {
		if group.Valid() {
			t.Errorf("Group %q is valid", group)
		}
	}
}
