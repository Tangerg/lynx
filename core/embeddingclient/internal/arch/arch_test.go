// Package arch holds architecture-fitness tests for the embeddingclient package.
package arch

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/scope/core/embeddingclient"
)

func TestClientSurfaceStaysVectorFocused(t *testing.T) {
	typeOfClient := reflect.TypeFor[embeddingclient.Client]()
	methods := make([]string, 0, typeOfClient.NumMethod())
	for method := range typeOfClient.Methods() {
		methods = append(methods, method.Name)
	}
	slices.Sort(methods)
	if !slices.Equal(methods, []string{"Dimensions", "EmbedDocuments", "EmbedText", "EmbedTexts"}) {
		t.Fatalf("Client methods = %v", methods)
	}
}
