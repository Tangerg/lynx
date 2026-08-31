// Package arch holds architecture-fitness tests for the embeddingclient package.
package arch

import (
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/embeddingclient"
)

func TestClientSurfaceStaysVectorFocused(t *testing.T) {
	typeOfClient := reflect.TypeFor[embeddingclient.Client]()
	for _, required := range []string{"EmbedText", "EmbedTexts"} {
		if _, exists := typeOfClient.MethodByName(required); !exists {
			t.Errorf("Client is missing %s", required)
		}
	}
	for _, forbidden := range []string{"Dimensions", "EmbedDocuments"} {
		if _, exists := typeOfClient.MethodByName(forbidden); exists {
			t.Errorf("Client exposes %s outside vector cardinality semantics", forbidden)
		}
	}
}
