package httpreq_test

import (
	"fmt"

	"github.com/Tangerg/scope/tools/httpreq"
)

func ExampleNewAllowlist() {
	allowlist, err := httpreq.NewAllowlist([]string{"api.example.com", "*.services.example.com"})
	if err != nil {
		panic(err)
	}

	fmt.Println(
		allowlist.Allows("API.EXAMPLE.COM"),
		allowlist.Allows("search.services.example.com"),
		allowlist.Allows("example.net"),
	)
	// Output:
	// true true false
}
