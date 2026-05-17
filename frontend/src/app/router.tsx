import { Suspense, lazy } from "react";
import type { ReactNode } from "react";
import { createBrowserRouter } from "react-router";
import { routeImports } from "./route-map";
import { AppShell } from "../components/layout/app-shell";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { RouteErrorState } from "../components/shared/route-error-state";
import { ErrorBoundary } from "../components/shared/error-boundary";

const DashboardPage = lazy(async () => routeImports["/"]().then((mod) => ({ default: mod.DashboardPage })));
const OverviewPage = lazy(async () => routeImports["/overview"]().then((mod) => ({ default: mod.OverviewPage })));
const ProcessesPage = lazy(async () => routeImports["/processes"]().then((mod) => ({ default: mod.ProcessesPage })));
const TreePage = lazy(async () => routeImports["/tree"]().then((mod) => ({ default: mod.TreePage })));
const PortsPage = lazy(async () => routeImports["/ports"]().then((mod) => ({ default: mod.PortsPage })));
const DisksPage = lazy(async () => routeImports["/disks"]().then((mod) => ({ default: mod.DisksPage })));
const AlertsPage = lazy(async () => routeImports["/alerts"]().then((mod) => ({ default: mod.AlertsPage })));
const RulesPage = lazy(async () => routeImports["/rules"]().then((mod) => ({ default: mod.RulesPage })));
const AIPage = lazy(async () => routeImports["/ai"]().then((mod) => ({ default: mod.AIPage })));
const SettingsPage = lazy(async () => routeImports["/settings"]().then((mod) => ({ default: mod.SettingsPage })));
const AboutPage = lazy(async () => routeImports["/about"]().then((mod) => ({ default: mod.AboutPage })));

function withSuspense(node: ReactNode) {
  return (
    <ErrorBoundary>
      <Suspense fallback={<PageSkeleton />}>{node}</Suspense>
    </ErrorBoundary>
  );
}

export const appRouter = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    errorElement: <RouteErrorState />,
    children: [
      { index: true, element: withSuspense(<DashboardPage />) },
      { path: "overview", element: withSuspense(<OverviewPage />) },
      { path: "processes", element: withSuspense(<ProcessesPage />) },
      { path: "tree", element: withSuspense(<TreePage />) },
      { path: "ports", element: withSuspense(<PortsPage />) },
      { path: "disks", element: withSuspense(<DisksPage />) },
      { path: "alerts", element: withSuspense(<AlertsPage />) },
      { path: "rules", element: withSuspense(<RulesPage />) },
      { path: "ai", element: withSuspense(<AIPage />) },
      { path: "settings", element: withSuspense(<SettingsPage />) },
      { path: "about", element: withSuspense(<AboutPage />) },
    ],
  },
]);
