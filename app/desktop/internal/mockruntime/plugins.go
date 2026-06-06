package agui

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Plugin sideload — serves user-installed plugin bundles from
// ~/.lyra/plugins/<id>/index.js.
//
// Each plugin is a directory containing at least `index.js` (ES module,
// default-exporting a definePlugin call). We don't transpile TS here; plugin
// authors who want JSX/TS pre-bundle with esbuild themselves. Keep the host
// dependency-free.

// PluginInfo is the JSON shape returned by GET /plugins.
type PluginInfo struct {
	ID  string `json:"id"`
	URL string `json:"url"` // path the frontend can dynamic-import
}

// pluginsDir returns the absolute path to the user plugin directory.
// Created on demand the first time anyone asks — so the user can drop files
// in without manually `mkdir`-ing.
func pluginsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lyra", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// listPlugins scans pluginsDir for `<id>/index.js` entries. Anything that
// doesn't have an index.js at the top level is silently skipped — keeps the
// behavior forgiving so users can stash README files etc. in there.
func listPlugins() ([]PluginInfo, error) {
	dir, err := pluginsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]PluginInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		idx := filepath.Join(dir, e.Name(), "index.js")
		if _, err := os.Stat(idx); err != nil {
			continue
		}
		out = append(out, PluginInfo{
			ID:  e.Name(),
			URL: "/plugins/" + e.Name() + "/index.js",
		})
	}
	return out, nil
}

func (s *Server) handlePluginsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infos, err := listPlugins()
	if err != nil {
		http.Error(w, "plugins: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(infos)
}

// handlePluginAsset serves files under ~/.lyra/plugins/<id>/...
//
// Security: path traversal is rejected. Only `.js` and `.css` are served — we
// don't want to expose arbitrary file types from the plugin dir (e.g. a stray
// `.env` file the user dropped in by mistake).
func (s *Server) handlePluginAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/plugins/")
	if rel == "" || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	ext := filepath.Ext(rel)
	if ext != ".js" && ext != ".css" && ext != ".mjs" {
		http.NotFound(w, r)
		return
	}

	dir, err := pluginsDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	full := filepath.Join(dir, filepath.FromSlash(rel))
	// Make sure the resolved path is still under the plugins dir.
	clean, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(clean, dir+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	// Set ESM content type so the browser's `import()` is happy.
	if ext == ".js" || ext == ".mjs" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, clean)
}
