module github.com/Tangerg/lynx/app/cli

go 1.26.5

require (
	github.com/Tangerg/lynx/app/tui v0.0.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
)

// The terminal interface library is developed in this repository and will move out on
// its own. Until it does it is resolved from the tree rather than from a version.
replace github.com/Tangerg/lynx/app/tui => ../tui
