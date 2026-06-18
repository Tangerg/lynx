import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/data/queryClient";
import { PluginProvider } from "@/plugins/host/PluginProvider";
import { AppRouter } from "@/router";

// Top-level providers. Order matters:
//   QueryClient   ── widest; plugins + queries both need it
//   PluginProvider ── inside QueryClient so plugin components can use queries
//   AppRouter      ── inside Plugins so routes can render plugin contributions
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <PluginProvider>
        <AppRouter />
      </PluginProvider>
    </QueryClientProvider>
  );
}

export default App;
