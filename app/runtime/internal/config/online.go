package config

import (
	"cmp"
	"strings"

	"github.com/spf13/viper"
)

// loadOnline reads the optional provider-tool credentials. yaml under
// `online:`; the SCOPEAPP_* env vars take precedence over yaml, matching
// the overall source ordering (env over file).
func loadOnline(v *viper.Viper) Online {
	jina := cmp.Or(jinaAPIKeyEnvironment.Value(), v.GetString("online.jinaApiKey"))
	tavily := cmp.Or(tavilyAPIKeyEnvironment.Value(), v.GetString("online.tavilyApiKey"))
	hosts := v.GetStringSlice("online.httpAllowedHosts")
	if env := httpHostsEnvironment.Value(); env != "" {
		hosts = splitHosts(env)
	}
	return Online{
		JinaAPIKey:       jina,
		TavilyAPIKey:     tavily,
		HTTPAllowedHosts: hosts,
	}
}

// splitHosts parses the comma-separated SCOPEAPP_HTTP_ALLOWED_HOSTS value.
func splitHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
