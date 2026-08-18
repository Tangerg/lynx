// The window's two resizable columns, as the chrome that renders them reads
// them: the drawer's collapse state plus each column's persisted width.
//
// The drawer state is the user's preference, full stop. Dock state cannot override it.

import { useDockWidth, useSidebarDrawer, useSidebarWidth } from "../application/navigation";

export { useDockWidth, useSidebarDrawer, useSidebarWidth };
