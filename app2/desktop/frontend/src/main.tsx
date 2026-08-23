import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { LocalizationProvider } from "./features/localization/Localization";
import { ShellPreferencesProvider } from "./features/preferences/ShellPreferences";
import { ToastProvider } from "./features/shell/ToastCenter";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 1_000, gcTime: 5 * 60_000 },
  },
});

const root = document.getElementById("root");
if (!root) throw new Error("Lyra root element is missing");

createRoot(root).render(
  <StrictMode>
    <ShellPreferencesProvider>
      <LocalizationProvider>
        <ToastProvider>
          <QueryClientProvider client={queryClient}>
            <App />
          </QueryClientProvider>
        </ToastProvider>
      </LocalizationProvider>
    </ShellPreferencesProvider>
  </StrictMode>,
);
