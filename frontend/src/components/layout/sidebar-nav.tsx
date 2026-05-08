import type { ComponentType } from "react";
import { Bot, Boxes, ChevronLeft, Cpu, GitBranch, HardDrive, Info, LayoutDashboard, Network, Settings, ShieldAlert, Sparkles, Workflow } from "lucide-react";
import { NavLink } from "react-router";
import type { AppRoutePath } from "../../app/route-map";
import { prefetchRoute } from "../../app/route-map";
import { cn } from "../../lib/cn";
import { useAlertsQuery } from "../../lib/api-client";

interface SidebarNavProps {
  collapsed?: boolean;
  onToggleCollapse?: () => void;
}

interface NavItem {
  to: AppRoutePath;
  label: string;
  icon: ComponentType<{ className?: string }>;
  badge?: number;
}

const baseNavItems: NavItem[] = [
  { to: "/", label: "Overview", icon: Cpu },
  { to: "/overview", label: "Metrics", icon: LayoutDashboard },
  { to: "/processes", label: "Processes", icon: Boxes },
  { to: "/tree", label: "Tree", icon: GitBranch },
  { to: "/ports", label: "Ports", icon: Network },
  { to: "/disks", label: "Disks", icon: HardDrive },
  { to: "/rules", label: "Rules", icon: Workflow },
  { to: "/ai", label: "AI", icon: Bot },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/about", label: "About", icon: Info },
];

export function SidebarNav({ collapsed = false, onToggleCollapse }: SidebarNavProps) {
  const { data } = useAlertsQuery();
  const activeCount = data?.active.length ?? 0;

  const navItems: NavItem[] = [
    ...baseNavItems,
    { to: "/alerts", label: "Alerts", icon: ShieldAlert, badge: activeCount },
  ];

  return (
    <div className="flex h-full flex-col">
      <div className={cn("border-b border-border px-3.5 py-4", collapsed && "px-2")}>
        <div className="flex items-center gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[0.65rem] border border-border bg-surface text-accent">
            <Sparkles className="h-3 w-3" />
          </div>
          {!collapsed && (
            <div>
              <div className="text-[0.68rem] font-medium uppercase tracking-[0.18em] text-secondary">Windows Task Manager</div>
              <div className="mt-0.5 text-[1.05rem] font-semibold tracking-tight text-foreground">WTM</div>
            </div>
          )}
        </div>
        {!collapsed && (
          <div className="mt-3 border-l-2 border-accent/45 pl-3 text-[0.8rem] leading-5 text-secondary">
            Local operator console for process triage, rules, alerts, ports, and guarded actions.
          </div>
        )}
      </div>
      <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-3">
        {navItems.map((item) => {
          const Icon = item.icon;
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              onMouseEnter={() => prefetchRoute(item.to)}
              className={({ isActive }) =>
                cn(
                  "flex min-h-9 items-center justify-between gap-2.5 rounded-[0.7rem] px-2.5 py-2 text-[0.88rem] font-medium text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-[var(--ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background",
                  "hover:bg-background-muted hover:text-foreground",
                  isActive && "bg-surface text-foreground ring-1 ring-border",
                  collapsed && "justify-center px-0",
                )
              }
            >
              <div className={cn("flex items-center gap-2.5", collapsed && "justify-center")}>
                <div className={cn("flex h-6.5 w-6.5 items-center justify-center rounded-[0.55rem] bg-background-muted text-secondary")}>
                  <Icon className="h-3.25 w-3.25 shrink-0" />
                </div>
                {!collapsed && <span>{item.label}</span>}
              </div>
              {!collapsed && item.badge !== undefined && item.badge > 0 && (
                <span className="min-w-5 rounded-full bg-error px-1.5 py-0.5 text-[0.65rem] font-bold text-accent-foreground text-center">
                  {item.badge > 99 ? "99+" : item.badge}
                </span>
              )}
              {collapsed && item.badge !== undefined && item.badge > 0 && (
                <span className="absolute right-1 top-1 h-2 w-2 rounded-full bg-error" />
              )}
            </NavLink>
          );
        })}
      </nav>
      <div className="border-t border-border px-3.5 py-3.5">
        <div className={cn("rounded-[0.8rem] border border-border bg-surface px-3 py-2.5", collapsed && "p-2")}>
          {!collapsed ? (
            <>
              <div className="eyebrow">Local service</div>
              <div className="mt-1.5 text-sm font-semibold text-foreground">localhost operator console</div>
              <div className="mt-1 text-[0.8rem] leading-5 text-secondary">Realtime stream first, polling fallback when transport blinks.</div>
            </>
          ) : (
            <div className="flex flex-col items-center gap-1">
              <div className="flex h-6 w-6 items-center justify-center rounded-[0.55rem] bg-background-muted text-secondary">
                <Sparkles className="h-3 w-3" />
              </div>
            </div>
          )}
        </div>
        {onToggleCollapse && (
          <button
            type="button"
            onClick={onToggleCollapse}
            className="mt-2 flex w-full items-center justify-center gap-2 rounded-[0.7rem] px-2.5 py-2 text-xs text-secondary hover:bg-background-muted hover:text-foreground transition-colors"
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <ChevronLeft className={cn("h-3.5 w-3.5 transition-transform", collapsed && "rotate-180")} />
            {!collapsed && <span>Collapse</span>}
          </button>
        )}
      </div>
    </div>
  );
}
