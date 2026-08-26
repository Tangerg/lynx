module github.com/Tangerg/lynx/app/runtime

go 1.27.0

require (
	github.com/Tangerg/lynx/core v0.0.0-20260826074033-2e35cbad116b
	github.com/Tangerg/lynx/a2a v0.0.0-20260825133344-d508117a2e44
	github.com/Tangerg/lynx/agent v0.0.0-20260826030651-a44943bd78f3
	github.com/Tangerg/lynx/app/runtime/localruntime v0.0.0-20260826074033-2e35cbad116b
	github.com/Tangerg/lynx/mcp v0.0.0-20260825131041-ced906387f71
	github.com/Tangerg/lynx/models/alibaba v0.0.0
	github.com/Tangerg/lynx/models/anthropic v0.0.0
	github.com/Tangerg/lynx/models/azureopenai v0.0.0
	github.com/Tangerg/lynx/models/catalog v0.0.0-20260826074033-2e35cbad116b
	github.com/Tangerg/lynx/models/deepseek v0.0.0
	github.com/Tangerg/lynx/models/fireworks v0.0.0
	github.com/Tangerg/lynx/models/google v0.0.0-20260826030651-a44943bd78f3
	github.com/Tangerg/lynx/models/groq v0.0.0
	github.com/Tangerg/lynx/models/huggingface v0.0.0
	github.com/Tangerg/lynx/models/minimax v0.0.0
	github.com/Tangerg/lynx/models/mistral v0.0.0
	github.com/Tangerg/lynx/models/moonshot v0.0.0
	github.com/Tangerg/lynx/models/openai v0.0.0
	github.com/Tangerg/lynx/models/openrouter v0.0.0
	github.com/Tangerg/lynx/models/perplexity v0.0.0
	github.com/Tangerg/lynx/models/protocol/openai v0.0.0-20260826030651-a44943bd78f3
	github.com/Tangerg/lynx/models/together v0.0.0
	github.com/Tangerg/lynx/models/xai v0.0.0
	github.com/Tangerg/lynx/models/xiaomi v0.0.0
	github.com/Tangerg/lynx/models/zhipu v0.0.0
	github.com/Tangerg/lynx/otel v0.0.0-20260826030651-a44943bd78f3
	github.com/Tangerg/lynx/skills v0.0.0-20260825142300-18eaf5d45cbb
	github.com/Tangerg/lynx/tools v0.0.0
	github.com/Tangerg/lynx/tools/httpreq v0.0.0-20260803213301-143b5c1045ad
	github.com/Tangerg/lynx/tools/skills v0.0.0-20260825142415-d09c68692be1
	github.com/Tangerg/lynx/tools/webfetch/jina v0.0.0
	github.com/Tangerg/lynx/tools/websearch/tavily v0.0.0
	github.com/Tangerg/sse v0.0.5
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-chi/chi/v5 v5.3.1
	github.com/go-chi/cors v1.2.2
	github.com/google/uuid v1.6.0
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/sourcegraph/jsonrpc2 v0.2.2
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/contrib/bridges/otelslog v0.19.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/log v0.20.0
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/log v0.20.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sys v0.47.0
	golang.org/x/tools v0.48.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.55.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.22.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/a2aproject/a2a-go/v2 v2.4.0 // indirect
	github.com/anthropics/anthropic-sdk-go v1.61.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.6.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-resty/resty/v2 v2.17.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.19 // indirect
	github.com/googleapis/gax-go/v2 v2.23.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/openai/openai-go/v3 v3.49.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/api v0.291.0 // indirect
	google.golang.org/genai v1.66.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260729162451-8efbd57d26e0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260729162451-8efbd57d26e0 // indirect
	google.golang.org/grpc v1.83.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/Tangerg/lynx/app/runtime/localruntime => ./localruntime
