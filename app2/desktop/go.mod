module github.com/Tangerg/lynx/app2/desktop

go 1.26.5

require (
	github.com/Tangerg/lynx/app2/runtime v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0-beta.12
	github.com/zalando/go-keyring v0.2.6
)

require (
	al.essio.dev/pkg/shellescape v1.6.0 // indirect
	github.com/Tangerg/sse v0.0.6 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modelcontextprotocol/go-sdk v1.7.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace (
	github.com/Tangerg/lynx => ../..
	github.com/Tangerg/lynx/agent => ../../agent
	github.com/Tangerg/lynx/app2/runtime => ../runtime
	github.com/Tangerg/lynx/models => ../../models
	github.com/Tangerg/lynx/models/google => ../../models/google
	github.com/Tangerg/lynx/models/protocol/openai => ../../models/protocol/openai
	github.com/Tangerg/lynx/skills => ../../skills
	github.com/Tangerg/lynx/tools/httpreq => ../../tools/httpreq
	github.com/Tangerg/lynx/tools/skills => ../../tools/skills
	github.com/Tangerg/lynx/tools/webfetch => ../../tools/webfetch
	github.com/Tangerg/lynx/tools/websearch => ../../tools/websearch
)
