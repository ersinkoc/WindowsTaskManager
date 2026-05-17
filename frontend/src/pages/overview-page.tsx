import { Cpu, Network } from "lucide-react";
import { Card } from "../components/ui/card";
import { PageHeader } from "../components/shared/page-header";
import { ChartSkeleton, DetailSkeleton } from "../components/shared/page-skeleton";
import { Badge } from "../components/ui/badge";
import { EmptyState } from "../components/shared/empty-state";
import { useSystemSnapshotQuery } from "../lib/api-client";
import { useSystemHistoryQuery } from "../lib/history-client";
import { formatBytes, formatPercent, formatRate } from "../lib/format";
import { Sparkline } from "../components/shared/sparkline";

export function OverviewPage() {
  const { data, isLoading } = useSystemSnapshotQuery();
  const { data: historyData } = useSystemHistoryQuery(120);

  const cpuHistory = (historyData?.history ?? []).map((d) => d.cpu?.total_percent ?? 0);
  const netDownHistory = (historyData?.history ?? []).map((d) => d.network?.total_down_bps ?? 0);
  const netUpHistory = (historyData?.history ?? []).map((d) => d.network?.total_up_bps ?? 0);

  if (isLoading) return (
    <div className="space-y-6">
      <PageHeader title="Overview" description="Per-core CPU breakdown, per-interface network traffic, and memory details at a glance." eyebrow="System metrics" icon={Cpu} />
      <ChartSkeleton />
      <DetailSkeleton />
      <ChartSkeleton />
    </div>
  );
  if (!data) return <EmptyState icon={Cpu} title="Waiting for live metrics" description="WTM collector is still warming up." />;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Overview"
        description="Per-core CPU breakdown, per-interface network traffic, and memory details at a glance."
        eyebrow="System metrics"
        icon={Cpu}
      />

      {/* Per-core CPU */}
      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <div>
            <h2 className="section-title">CPU cores</h2>
            <p className="mt-1 text-sm text-secondary">
              {data.cpu.name} &middot; {data.cpu.num_logical} logical cores
            </p>
          </div>
          <div className="flex items-center gap-3">
            {cpuHistory.length > 1 && <Sparkline data={cpuHistory} width={100} height={30} />}
            <Badge variant="neutral">{data.cpu.total_percent.toFixed(1)}% total</Badge>
          </div>
        </div>
        <div className="grid gap-3 p-4 sm:px-5 sm:py-4">
          <div className="grid gap-2" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(11rem, 1fr))" }}>
            {data.cpu.per_core.map((percent, i) => (
              <CoreBar key={i} index={i} percent={percent} />
            ))}
          </div>
        </div>
      </Card>

      {/* Memory */}
      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <div>
            <h2 className="section-title">Memory</h2>
            <p className="mt-1 text-sm text-secondary">
              {formatBytes(data.memory.used_phys)} used of {formatBytes(data.memory.total_phys)}
            </p>
          </div>
          <Badge variant={data.memory.used_percent > 85 ? "error" : data.memory.used_percent > 70 ? "warning" : "success"}>
            {formatPercent(data.memory.used_percent)}
          </Badge>
        </div>
        <div className="grid gap-4 p-4 sm:px-5 sm:py-4">
          {/* Visual bar */}
          <div className="space-y-2">
            <div className="meter h-3.5 w-full">
              <div
                className="meter-bar"
                style={{ width: `${Math.min(100, data.memory.used_percent)}%` }}
              />
            </div>
            <div className="flex justify-between text-xs text-secondary">
              <span>{formatBytes(data.memory.used_phys)} used</span>
              <span>{formatBytes(data.memory.total_phys - data.memory.used_phys)} available</span>
            </div>
          </div>
        </div>
      </Card>

      {/* Network interfaces */}
      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <div>
            <h2 className="section-title">Network interfaces</h2>
            <p className="mt-1 text-sm text-secondary">
              {data.network.interfaces.length} interface{data.network.interfaces.length !== 1 ? "s" : ""} &middot;{" "}
              ↓{formatRate(data.network.total_down_bps)} &nbsp; ↑{formatRate(data.network.total_up_bps)}
            </p>
          </div>
          <div className="flex items-center gap-3">
            {netDownHistory.length > 1 && (
              <div className="flex items-center gap-1.5">
                <span className="text-[0.65rem] text-secondary">↓</span>
                <Sparkline data={netDownHistory} width={80} height={22} />
              </div>
            )}
            {netUpHistory.length > 1 && (
              <div className="flex items-center gap-1.5">
                <span className="text-[0.65rem] text-secondary">↑</span>
                <Sparkline data={netUpHistory} width={80} height={22} />
              </div>
            )}
            <Badge variant="neutral">{data.network.interfaces.filter((i) => i.status === "up").length} up</Badge>
          </div>
        </div>
        <div className="grid gap-3 p-4 sm:px-5 sm:py-4">
          {data.network.interfaces.length === 0 && (
            <p className="py-4 text-center text-sm text-secondary">No network interfaces detected.</p>
          )}
          <div className="grid gap-3" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(20rem, 1fr))" }}>
            {data.network.interfaces.map((iface) => (
              <InterfaceCard key={iface.name} iface={iface} />
            ))}
          </div>
        </div>
      </Card>
    </div>
  );
}

function CoreBar({ index, percent }: { index: number; percent: number }) {
  const color =
    percent >= 90 ? "bg-error" :
    percent >= 75 ? "bg-warning" :
    percent >= 50 ? "bg-accent" :
    "bg-success";
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-secondary">Core {index}</span>
        <span className="font-mono text-foreground">{percent.toFixed(1)}%</span>
      </div>
      <div className="meter h-2.5">
        <div
          className={`meter-bar ${color}`}
          style={{ width: `${Math.min(100, percent)}%` }}
        />
      </div>
    </div>
  );
}

interface InterfaceCardProps {
  iface: {
    name: string;
    type: string;
    status: string;
    speed_mbps: number;
    in_bps: number;
    out_bps: number;
  };
}

function InterfaceCard({ iface }: InterfaceCardProps) {
  const isUp = iface.status === "up";
  return (
    <div className="soft-panel">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Network className="h-4 w-4 shrink-0 text-accent" />
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-foreground">{iface.name}</div>
            <div className="mt-0.5 flex items-center gap-2 text-xs text-secondary">
              {iface.type && <span>{iface.type}</span>}
              {iface.speed_mbps > 0 && (
                <span className="font-mono">{iface.speed_mbps} Mbps</span>
              )}
            </div>
          </div>
        </div>
        <Badge variant={isUp ? "success" : "neutral"}>
          {isUp ? "Up" : "Down"}
        </Badge>
      </div>

      {isUp && (
        <div className="mt-3 grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <div className="text-[0.65rem] font-semibold uppercase tracking-wider text-secondary">
              <span className="inline-block mr-1">↓</span>Download
            </div>
            <div className="text-sm font-semibold tabular-nums text-foreground">
              {formatRate(iface.in_bps)}
            </div>
          </div>
          <div className="space-y-1">
            <div className="text-[0.65rem] font-semibold uppercase tracking-wider text-secondary">
              <span className="inline-block mr-1">↑</span>Upload
            </div>
            <div className="text-sm font-semibold tabular-nums text-foreground">
              {formatRate(iface.out_bps)}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
