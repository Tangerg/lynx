package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// How the frontend addresses this host. v3's binding engine builds each name by
// reflection at startup — package path, type name, method name, joined by dots
// (application/bindings.go) — so a name that does not match is not a compile error on
// either side. It is a runtime "unknown bound method", after the window is already up, on
// a call the app makes once during boot and never retries.
const hostPackage = "main"

// The package qualifier is a LITERAL and must stay one. Deriving it from
// `reflect.TypeOf(&DesktopHost{}).Elem().PkgPath()` looks obviously better and is wrong:
// a test binary reports the module's import path (github.com/Tangerg/lynx/app/desktop)
// while the built application reports "main", so a test that derives it disagrees with
// the app it is checking — and the disagreement points the wrong way. Deriving it here
// once reported the frontend as broken and would have had it "fixed" to a name the real
// binary never registers. Verified in both directions with a minimal module: `go build`
// yields "main", `go test` yields the module path.
func TestDesktopHostMethodNamesMatchTheFrontend(t *testing.T) {
	hostType := reflect.TypeFor[*DesktopHost]()

	source, err := os.ReadFile("frontend/src/rpc/desktopHost.ts")
	if err != nil {
		t.Fatalf("read the frontend's host bridge: %v", err)
	}

	for _, method := range []string{"Bootstrap", "WindowChrome"} {
		if _, ok := hostType.MethodByName(method); !ok {
			t.Errorf("DesktopHost has no exported %s method to bind", method)
			continue
		}
		fqn := fmt.Sprintf("%s.%s.%s", hostPackage, hostType.Elem().Name(), method)
		if !strings.Contains(string(source), `"`+fqn+`"`) {
			t.Errorf("the frontend does not name %q; it cannot call what it cannot address", fqn)
		}
	}
}
