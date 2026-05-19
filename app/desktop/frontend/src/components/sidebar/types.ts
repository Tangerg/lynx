// Sidebar component types — kept small and independent of the AG-UI / mock
// layers so the sidebar can be reused with any session source.

export type SidebarSession = {
  id: string;
  title: string;
  status: "running" | "waiting" | "idle";
  model: string;
  time: string;
};

export type SidebarProject = {
  id: string;
  name: string;
  branch: string;
  active?: boolean;
};

export type Theme = "dark" | "light";
