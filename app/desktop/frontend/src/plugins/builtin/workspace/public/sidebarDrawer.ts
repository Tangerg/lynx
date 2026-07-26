// The window's two resizable columns, as the chrome that renders them reads
// them: the drawer's collapse state plus each column's persisted width.
//
// The drawer state is the user's preference, full stop. It used to be OR-ed with
// "a dock view is open", so opening one silently overrode the preference and the
// toggle button became inert — a control that lies about what it does.

import { useDockWidth, useSidebarDrawer, useSidebarWidth } from "../application/navigation";

export { useDockWidth, useSidebarDrawer, useSidebarWidth };
