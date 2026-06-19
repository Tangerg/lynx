import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { queryClient } from "@/lib/data/queryClient";
import { PluginProvider } from "@/plugins/host/PluginProvider";
import { AppRouter } from "@/router";

// Top-level providers. Order matters:
//   QueryClient   ── widest; plugins + queries both need it
//   MotionConfig  ── reducedMotion="user" makes every motion/react animation
//                    honor the OS "reduce motion" setting (transform/scale
//                    collapse to opacity). The CSS @media rule already covers
//                    CSS transitions; this extends the same respect to the JS
//                    animation half, which it otherwise misses.
//   PluginProvider ── inside QueryClient so plugin components can use queries
//   AppRouter      ── inside Plugins so routes can render plugin contributions
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <PluginProvider>
          <AppRouter />
        </PluginProvider>
      </MotionConfig>
    </QueryClientProvider>
  );
}

export default App;
