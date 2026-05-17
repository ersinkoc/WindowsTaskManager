import { Clock3, Wifi, WifiOff } from "lucide-react";
import { useMemo } from "react";
import { useLocation } from "react-router";
import { useSystemSnapshotQuery } from "../../lib/api-client";
import { formatBytes } from "../../lib/format";
import { useUIStore } from "../../stores/ui-store";
import { Badge } from "../ui/badge";
import { ThemeMenu } from "./theme-menu";

const pageTitles: Record<string, string> = {
  "/": "Overview",
  "/processes": "Processes",
  "/tree": "Process Tree",
  "/ports": "Ports",
  "/disks": "Disks",
  "/alerts": "Alerts",
  "/rules": "Rules",
  "/ai": "AI Advisor",
  "/about": "About",
};

export function Topbar() {
  const location = useLocation();
  const { data } = useSystemSnapshotQuery();
  const streamConnected = useUIStore((state) => state.streamConnected);
  const pageTitle = pageTitles[location.pathname] ?? "Dashboard";
  const cpuText = useMemo(() => `${data?.cpu.total_percent.toFixed(1) ?? "0.0"}% CPU`, [data]);
  const memoryText = useMemo(() => {
    if (!data) {
      return "0 GB used";
    }
    return `${formatBytes(data.memory.used_phys)} used`;
  }, [data]);
  const timestampText = useMemo(() => {
    if (!data?.timestamp) {
      return "Waiting for snapshot";
    }
    return new Date(data.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  }, [data?.timestamp]);

  return (
    <div className="flex w-full flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-1">
          <div className="flex flex-col">
            <div className="eyebrow">WTM command surface</div>
            <h1 className="mt-0.5 text-[1.05rem] font-semibold tracking-tight text-foreground">{pageTitle}</h1>
          </div>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 lg:justify-end">
        <Badge variant="neutral">{cpuText}</Badge>
        <Badge variant="neutral">{memoryText}</Badge>
        <Badge variant={streamConnected ? "success" : "warning"}>
          {streamConnected ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
          {streamConnected ? "Streaming" : "Polling"}
        </Badge>
        <Badge variant="neutral">
          <Clock3 className="h-3.5 w-3.5" />
          {timestampText}
        </Badge>
        <ThemeMenu />
      </div>
    </div>
  );
}
