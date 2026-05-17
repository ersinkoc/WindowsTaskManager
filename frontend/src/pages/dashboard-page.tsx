import type { ComponentType } from "react";
import { Activity, ArrowDownRight, Cpu, MemoryStick, Network, ShieldAlert } from "lucide-react";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { Badge } from "../components/ui/badge";
import { Card } from "../components/ui/card";
import { useAlertsQuery, useSystemSnapshotQuery } from "../lib/api-client";
import { useSystemHistoryQuery } from "../lib/history-client";
import { formatBytes, formatPercent, formatRate } from "../lib/format";
import { meterBarClassName } from "../lib/meters";
import { Sparkline } from "../components/shared/sparkline";
import type { PortBinding, ProcessInfo, SystemSnapshot } from "../types/api";

export function DashboardPage() {
  const { data, isLoading } = useSystemSnapshotQuery();
  const { data: alertsData } = useAlertsQuery();
  const { data: historyData } = useSystemHistoryQuery(60);

  const history = historyData?.history ?? [];
  const cpuHistory = history.map((d) => d.cpu?.total_percent ?? 0);
  const memHistory = history.map((d) => d.memory?.used_percent ?? 0);

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (!data) {
    return <EmptyState icon={Activity} title="Waiting for live metrics" description="WTM is still warming up the local collector and snapshot stream." />;
  }

  const topProcesses = [...data.processes].sort((left, right) => {
    if (right.cpu_percent !== left.cpu_percent) {
      return right.cpu_percent - left.cpu_percent;
    }
    if (right.working_set !== left.working_set) {
      return right.working_set - left.working_set;
    }
    return left.name.localeCompare(right.name);
  });
  const busiestProcess = topProcesses[0] ?? null;
  const heaviestProcess = [...data.processes].sort((left, right) => right.working_set - left.working_set)[0] ?? null;
  const activeAlerts = alertsData?.active ?? [];
  const criticalAlerts = activeAlerts.filter((alert) => alert.severity === "critical").length;
  const topBindings = topPortBindings(data.port_bindings ?? []);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        description="A compact machine summary: load, alerts, top offenders, and the endpoints worth checking."
        eyebrow="Overview"
        icon={Activity}
        meta={
          <>
            <Badge variant="info">{data.processes.length} processes</Badge>
            <Badge variant={criticalAlerts > 0 ? "error" : activeAlerts.length > 0 ? "warning" : "success"}>
              {criticalAlerts > 0 ? `${criticalAlerts} critical alerts` : activeAlerts.length > 0 ? `${activeAlerts.length} active alerts` : "No active alerts"}
            </Badge>
          </>
        }
      />

      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:px-5 xl:flex-row xl:items-end xl:justify-between">
          <div className="max-w-3xl">
            <div className="eyebrow">System pulse</div>
            <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground sm:text-[1.45rem]">{healthHeadline(data, activeAlerts.length)}</h2>
            <p className="mt-1 text-sm leading-6 text-secondary">
              CPU is at {formatPercent(data.cpu.total_percent)}, memory at {formatPercent(data.memory.used_percent)}, and the network is moving{" "}
              {formatRate(data.network.total_down_bps)} down right now.
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-3 xl:min-w-[31rem]">
            <QuickFact
              label="Busiest process"
              value={busiestProcess ? busiestProcess.name : "--"}
              detail={busiestProcess ? `${busiestProcess.cpu_percent.toFixed(1)}% CPU` : "Waiting"}
            />
            <QuickFact
              label="Heaviest memory"
              value={heaviestProcess ? heaviestProcess.name : "--"}
              detail={heaviestProcess ? formatBytes(heaviestProcess.working_set) : "Waiting"}
            />
            <QuickFact
              label="Immediate watch"
              value={criticalAlerts > 0 ? `${criticalAlerts} critical` : `${activeAlerts.length} active`}
              detail={criticalAlerts > 0 ? "Needs action" : "Stable"}
            />
          </div>
        </div>
      </Card>

      <div className="stat-grid">
        <MetricCard icon={Cpu} label="CPU" value={formatPercent(data.cpu.total_percent)} detail={data.cpu.name || "Processor"} meter={data.cpu.total_percent} sparkline={cpuHistory} />
        <MetricCard
          icon={MemoryStick}
          label="Memory"
          value={formatPercent(data.memory.used_percent)}
          detail={`${formatBytes(data.memory.used_phys)} / ${formatBytes(data.memory.total_phys)}`}
          meter={data.memory.used_percent}
          sparkline={memHistory}
        />
        <MetricCard
          icon={Activity}
          label="GPU"
          value={data.gpu.available ? `${data.gpu.utilization.toFixed(0)}%` : "n/a"}
          detail={data.gpu.name || "GPU unavailable"}
          meter={data.gpu.available ? data.gpu.utilization : undefined}
        />
        <MetricCard
          icon={Network}
          label="Network"
          value={formatRate(data.network.total_down_bps)}
          detail={`Up ${formatRate(data.network.total_up_bps)}`}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.65fr)_minmax(18rem,0.8fr)]">
        <Card className="space-y-0 overflow-hidden p-0">
          <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
            <div>
              <h2 className="section-title">Top processes</h2>
              <p className="mt-1 text-sm text-secondary">Fast scan of the hottest processes before you open the full process view.</p>
            </div>
            <Badge variant="info">Live ranking</Badge>
          </div>

          <div className="grid gap-3 p-4 sm:hidden sm:p-5">
            {topProcesses.slice(0, 5).map((process) => (
              <TopProcessCard key={process.pid} process={process} />
            ))}
          </div>

          <div className="hidden sm:block">
            <table className="dense-table min-w-full table-fixed text-left text-sm">
              <thead className="border-b border-border bg-background/55">
                <tr>
                  <th className="w-18 px-4 py-3 pr-3 sm:px-5">PID</th>
                  <th className="py-3 pr-3">Name</th>
                  <th className="w-18 py-3 pr-3">CPU</th>
                  <th className="w-24 py-3 pr-3">Memory</th>
                  <th className="w-18 py-3 pr-4 sm:pr-5">Threads</th>
                </tr>
              </thead>
              <tbody>
                {topProcesses.slice(0, 10).map((process) => (
                  <tr key={process.pid} className="border-b border-border transition-colors hover:bg-background/55">
                    <td className="px-4 py-2.5 pr-3 font-mono text-secondary whitespace-nowrap sm:px-5">{process.pid}</td>
                    <td className="py-2.5 pr-3 text-foreground">
                      <div className="truncate font-medium">{process.name}</div>
                    </td>
                    <td className="py-2.5 pr-3 text-secondary whitespace-nowrap">{process.cpu_percent.toFixed(1)}%</td>
                    <td className="py-2.5 pr-3 text-secondary whitespace-nowrap">{formatBytes(process.working_set)}</td>
                    <td className="py-2.5 pr-4 text-secondary whitespace-nowrap sm:pr-5">{process.thread_count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>

        <Card className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="section-title">Attention board</h2>
            <Badge variant={criticalAlerts > 0 ? "error" : "neutral"}>{criticalAlerts > 0 ? "Escalated" : "Calm"}</Badge>
          </div>
          <div className="grid gap-2">
            <FocusRow label="Alerts" value={criticalAlerts > 0 ? `${criticalAlerts} critical` : `${activeAlerts.length} active`} detail="Anomaly engine output." />
            <FocusRow label="Ports" value={String(topBindings.length)} detail="Highest-interest live bindings." />
            <FocusRow
              label="Collector"
              value={data.gpu.available ? "GPU online" : "GPU optional"}
              detail={data.gpu.available ? data.gpu.name || "Graphics telemetry live" : "CPU, memory, disk, network still active"}
            />
          </div>
          {activeAlerts.length > 0 ? (
            <div className="grid gap-2">
              {activeAlerts.slice(0, 3).map((alert) => (
                <div key={`${alert.type}-${alert.pid ?? "global"}-${alert.title}`} className="soft-panel">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={alert.severity === "critical" ? "error" : alert.severity === "warning" ? "warning" : "info"}>{alert.severity}</Badge>
                    {alert.pid ? <Badge variant="neutral">PID {alert.pid}</Badge> : null}
                    <div className="truncate text-sm font-semibold text-foreground">{alert.title}</div>
                  </div>
                  <div className="mt-1.5 text-sm leading-5 text-secondary">{alert.description}</div>
                </div>
              ))}
            </div>
          ) : null}
          <div className="grid gap-2">
            {topBindings.slice(0, 3).map((binding) => (
              <div key={bindingKey(binding)} className="soft-panel">
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-foreground">{binding.process || binding.label || "Unknown process"}</div>
                    <div className="mt-0.5 truncate font-mono text-xs text-secondary">
                      {binding.local_addr}:{binding.local_port}
                    </div>
                  </div>
                  <Badge variant={binding.state === "LISTEN" ? "success" : "info"}>{binding.state || "OPEN"}</Badge>
                </div>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}

interface MetricCardProps {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
  detail: string;
  meter?: number;
  sparkline?: number[];
}

function MetricCard({ icon: Icon, label, value, detail, meter, sparkline }: MetricCardProps) {
  return (
    <Card>
      <div className="flex items-start gap-3">
        <div className="mt-0.5 rounded-md border border-border bg-background px-2.5 py-2 text-accent">
          <Icon className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="metric-label">{label}</div>
          <div className="mt-1.5 overflow-hidden text-ellipsis whitespace-nowrap text-[1.65rem] font-semibold tracking-tight text-foreground sm:text-[1.9rem]">{value}</div>
          <div className="mt-0.5 text-sm leading-6 text-secondary">{detail}</div>
          {typeof meter === "number" ? (
            <div className="meter">
              <div className={meterBarClassName(meter)} />
            </div>
          ) : null}
        </div>
        {sparkline && sparkline.length > 1 && (
          <div className="mt-1 shrink-0">
            <Sparkline data={sparkline} width={100} height={36} />
          </div>
        )}
      </div>
    </Card>
  );
}

interface TopProcessCardProps {
  process: ProcessInfo;
}

function TopProcessCard({ process }: TopProcessCardProps) {
  return (
    <div className="soft-panel">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-foreground">{process.name}</div>
          <div className="mt-1 font-mono text-xs text-secondary">PID {process.pid}</div>
        </div>
        <div className="flex items-center gap-1 text-sm font-semibold text-foreground">
          <ArrowDownRight className="h-4 w-4 text-accent" />
          {process.cpu_percent.toFixed(1)}%
        </div>
      </div>
      <div className="mt-2.5 grid grid-cols-2 gap-2 text-sm">
        <QuickFact label="Memory" value={formatBytes(process.working_set)} detail="Working set" compact />
        <QuickFact label="Threads" value={String(process.thread_count)} detail="Live threads" compact />
      </div>
    </div>
  );
}

interface QuickFactProps {
  label: string;
  value: string;
  detail: string;
  compact?: boolean;
}

function QuickFact({ label, value, detail, compact = false }: QuickFactProps) {
  return (
    <div className={compact ? "soft-panel" : "soft-panel min-w-[11rem]"}>
      <div className="metric-label">{label}</div>
      <div className="mt-1 truncate text-base font-semibold text-foreground">{value}</div>
      <div className="mt-0.5 text-xs text-secondary">{detail}</div>
    </div>
  );
}

interface FocusRowProps {
  label: string;
  value: string;
  detail: string;
}

function FocusRow({ label, value, detail }: FocusRowProps) {
  return (
    <div className="soft-panel">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="metric-label">{label}</div>
          <div className="mt-1 text-base font-semibold text-foreground">{value}</div>
          <div className="mt-1 text-sm leading-5 text-secondary">{detail}</div>
        </div>
        <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-accent" />
      </div>
    </div>
  );
}

function healthHeadline(data: SystemSnapshot, activeAlerts: number) {
  if (activeAlerts > 0) {
    return "The machine is running, but it wants attention.";
  }
  if (data.cpu.total_percent >= 85 || data.memory.used_percent >= 90) {
    return "Resource pressure is climbing fast.";
  }
  if (data.cpu.total_percent >= 55 || data.memory.used_percent >= 70) {
    return "The box is busy, but still under control.";
  }
  return "Everything looks calm and readable.";
}

function topPortBindings(bindings: PortBinding[]) {
  return [...bindings]
    .sort((left, right) => {
      if (left.state === "LISTEN" && right.state !== "LISTEN") {
        return -1;
      }
      if (left.state !== "LISTEN" && right.state === "LISTEN") {
        return 1;
      }
      return right.local_port - left.local_port;
    })
    .slice(0, 4);
}

function bindingKey(binding: PortBinding) {
  return `${binding.protocol}-${binding.pid}-${binding.local_addr}-${binding.local_port}-${binding.remote_addr}-${binding.remote_port}-${binding.state}`;
}
