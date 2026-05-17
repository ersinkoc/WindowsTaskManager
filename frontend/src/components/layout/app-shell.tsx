import { Outlet } from "react-router";
import { useUIStore } from "../../stores/ui-store";
import { NetworkBanner } from "../shared/network-banner";
import { LiveStrip } from "./live-strip";
import { SidebarNav } from "./sidebar-nav";
import { Topbar } from "./topbar";

export function AppShell() {
  const sidebarCollapsed = useUIStore((state) => state.sidebarCollapsed);
  const setSidebarCollapsed = useUIStore((state) => state.setSidebarCollapsed);

  return (
    <div className="app-shell">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[600] focus:rounded-xl focus:bg-accent focus:px-4 focus:py-3 focus:text-accent-foreground"
      >
        Skip to main content
      </a>
      <aside
        className={[
          "fixed inset-y-0 left-0 z-[200] border-r border-border bg-background-subtle/98 backdrop-blur-md transition-all duration-150",
          "w-60 hidden lg:block",
        ].join(" ")}
      >
        <SidebarNav collapsed={sidebarCollapsed} onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)} />
      </aside>
      <main
        id="main-content"
        className="min-h-screen overflow-y-auto transition-all duration-150 lg:pl-60"
        tabIndex={-1}
      >
        <header className="sticky top-0 z-[100] border-b border-border bg-background/88 backdrop-blur-md">
          <div className="page-padding flex min-h-14 items-center gap-3 py-2">
            <Topbar />
          </div>
        </header>
        <NetworkBanner />
        <LiveStrip />
        <div className="page-padding py-4 sm:py-5 lg:py-5">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
