import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { LocalizationProvider } from "../features/localization/Localization";
import { ShellPreferencesProvider } from "../features/preferences/ShellPreferences";
import { VisualApp } from "./VisualApp";
import "../styles.css";
import "../styles/foundation.css";
import "../styles/shell.css";
import "../styles/settings.css";

const root = document.getElementById("root");
if (root === null) throw new Error("Visual acceptance root is missing");

const query = new URLSearchParams(window.location.search);
const theme = query.get("theme") === "graphite" ? "graphite" : "linen";
const locale = query.get("locale") === "ar" ? "ar" : "en";
window.localStorage.setItem(
  "lyra.app2.shell.v3",
  JSON.stringify({ theme, accent: "ocean", locale }),
);

createRoot(root).render(
  <StrictMode>
    <ShellPreferencesProvider>
      <LocalizationProvider>
        <VisualApp
          initialSurface={
            query.get("surface") === "settings" ? "settings" : "workspace"
          }
          sidebarOpen={query.get("sidebar") !== "closed"}
          dockOpen={query.get("dock") === "open"}
        />
      </LocalizationProvider>
    </ShellPreferencesProvider>
  </StrictMode>,
);
