package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestWailsServicesExposeOnlyReviewedMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		typeOf reflect.Type
		want   []string
	}{
		{
			name:   "DesktopHost",
			typeOf: reflect.TypeFor[*DesktopHost](),
			want: []string{
				"Bootstrap",
				"ConnectRemoteRuntime",
				"ForgetRemoteRuntime",
				"RemoteRuntime",
				"UseLocalRuntime",
				"UseRemoteRuntime",
			},
		},
		{
			name:   "NativeHost",
			typeOf: reflect.TypeFor[*NativeHost](),
			want: []string{
				"ChooseDirectory",
				"OpenSessionArtifact",
				"SaveImage",
				"SaveSessionExport",
				"WindowChrome",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			methods := make([]string, 0, test.typeOf.NumMethod())
			for index := range test.typeOf.NumMethod() {
				methods = append(methods, test.typeOf.Method(index).Name)
			}
			slices.Sort(methods)
			if !slices.Equal(methods, test.want) {
				t.Fatalf("exported methods = %v, want %v", methods, test.want)
			}
		})
	}
}
