import { useEffect, useMemo, useState, useRef } from "react";
import { ArrowDown, ArrowUp, Boxes, PlugZap, ShieldBan, X } from "lucide-react";
import { useNavigate, useSearchParams } from "react-router";
import { DetailTile, SummaryCard } from "../components/shared/detail-tile";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { ConfirmDialog } from "../components/shared/confirm-dialog";
import { SearchInput } from "../components/shared/search-input";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { useInfoQuery, useProcessActionMutation, useProcessConnectionsQuery, useSystemSnapshotQuery } from "../lib/api-client";
import { formatBytes } from "../lib/format";
import type { PortBinding, ProcessInfo } from "../types/api";
import { createPortal } from "react-dom";

type ProcessSortKey = "cpu" | "memory" | "name" | "threads" | "connections" | "pid";
type SortDirection = "desc" | "asc";

export function ProcessesPage() {
  const { data, isLoading } = useSystemSnapshotQuery();
  const { data: info } = useInfoQuery();
  const navigate = useNavigate();
  const actionMutation = useProcessActionMutation({ successMessage: false });
  const [killCandidate, setKillCandidate] = useState<ProcessInfo | null>(null);
  const [detailProcess, setDetailProcess] = useState<ProcessInfo | null>(null);
  const [searchValue, setSearchValue] = useState("");
  const [sortKey, setSortKey] = useState<ProcessSortKey>("cpu");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const tableRef = useRef<HTMLDivElement>(null);

  // PID filter from URL (e.g. ?pid=1234 from clicking a PID in Ports/Alerts/Tree)
  const urlPID = searchParams.get("pid");
  const urlPIDNum = urlPID ? Number(urlPID) : null;
  const debouncedSearch = useDebouncedValue(searchValue, 300);
  const processes = data?.processes ?? [];
  const selfPID = info?.self_pid ?? null;
  const portCountByPID = useMemo(() => {
    const counts = new Map<number, number>();
    for (const binding of data?.port_bindings ?? []) {
      counts.set(binding.pid, (counts.get(binding.pid) ?? 0) + 1);
    }
    return counts;
  }, [data?.port_bindings]);

  const filteredProcesses = useMemo(() => {
    const enriched = processes.map((process) => ({
      ...process,
      connections: portCountByPID.get(process.pid) ?? process.connections,
    }));
    const needle = debouncedSearch.trim().toLowerCase();
    let base = enriched;

    // PID filter from URL (when navigating from Ports/Alerts/Tree)
    if (urlPIDNum !== null) {
      base = base.filter((p) => p.pid === urlPIDNum);
    }

    if (needle) {
      base = base.filter((process) => {
        return process.name.toLowerCase().includes(needle) || String(process.pid).includes(needle);
      });
    }

    const sorted = [...base];
    sorted.sort((left, right) => compareProcesses(left, right, sortKey, sortDirection));
    return sorted;
  }, [debouncedSearch, portCountByPID, processes, sortDirection, sortKey, urlPIDNum]);

  const topProcess = filteredProcesses[0] ?? null;
  const memoryLeader = [...filteredProcesses].sort((left, right) => right.working_set - left.working_set)[0] ?? null;
  const detailPID = detailProcess?.pid ?? null;
  const { data: processConnections = [], isLoading: connectionsLoading } = useProcessConnectionsQuery(detailPID);
  const portSummary = useMemo(() => summarizePorts(processConnections), [processConnections]);

  // Keyboard navigation: j/k to move, Enter to select, Escape to close
  useEffect(() => {
    if (detailProcess || killCandidate) return;
    const handler = (e: KeyboardEvent) => {
      const rows = tableRef.current?.querySelectorAll<HTMLTableRowElement>("tbody tr");
      if (!rows || rows.length === 0) return;
      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, rows.length - 1));
      } else if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        const row = rows[selectedIndex >= 0 ? selectedIndex : 0];
        if (row) row.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [detailProcess, killCandidate, selectedIndex]);

  const updateSort = (nextKey: ProcessSortKey) => {
    if (sortKey === nextKey) {
      setSortDirection((current) => (current === "desc" ? "asc" : "desc"));
      return;
    }
    setSortKey(nextKey);
    setSortDirection("desc");
  };

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (!data) {
    return <EmptyState icon={Boxes} title="No process data yet" description="Waiting for the process list from /api/v1/system." />;
  }

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="Processes"
          description="Sort, inspect, and act on live processes without the surrounding noise."
          eyebrow="Control room"
          icon={Boxes}
          meta={
            <>
              {urlPIDNum !== null && (
                <button
                  type="button"
                  className="inline-flex items-center gap-1.5 rounded-md border border-accent bg-accent-muted/45 px-2 py-0.5 text-xs font-semibold text-accent transition-colors hover:bg-accent-muted"
                  onClick={() => setSearchParams({})}
                  title="Clear PID filter"
                >
                  PID {urlPIDNum} <X className="h-3 w-3" />
                </button>
              )}
              <Badge variant="info">{filteredProcesses.length} visible</Badge>
              <Badge variant={topProcess && topProcess.cpu_percent >= 80 ? "warning" : "success"}>
                {topProcess ? `${topProcess.name} leads CPU` : "Stable"}
              </Badge>
            </>
          }
          actions={
            <>
              <SearchInput
                ariaLabel="Search processes by name or PID"
                placeholder="Search process or PID"
                value={searchValue}
                onChange={setSearchValue}
              />
              <div className="flex gap-2 md:hidden">
                <label className="flex min-h-9 flex-1 items-center gap-2 rounded-lg border border-border bg-background px-3 text-sm text-secondary">
                  <span>Sort</span>
                  <select
                    aria-label="Sort processes"
                    className="min-w-0 flex-1 bg-transparent text-foreground"
                    value={sortKey}
                    onChange={(event) => updateSort(event.target.value as ProcessSortKey)}
                  >
                    <option value="cpu">CPU</option>
                    <option value="memory">Memory</option>
                    <option value="name">Name</option>
                    <option value="threads">Threads</option>
                    <option value="connections">Ports</option>
                    <option value="pid">PID</option>
                  </select>
                </label>
                <Button type="button" variant="secondary" onClick={() => setSortDirection((current) => (current === "desc" ? "asc" : "desc"))}>
                  {sortDirection === "desc" ? <ArrowDown className="h-4 w-4" /> : <ArrowUp className="h-4 w-4" />}
                </Button>
              </div>
            </>
          }
        />

        <div className="grid gap-3 lg:grid-cols-3">
          <SummaryCard label="Visible processes" value={String(filteredProcesses.length)} accent={<Badge variant="info">Filtered</Badge>} />
          <SummaryCard
            label="Top CPU now"
            value={topProcess ? `${topProcess.name} (${topProcess.cpu_percent.toFixed(1)}%)` : "--"}
            valueClassName="text-base font-semibold sm:text-lg"
            accent={<Badge variant="warning">CPU</Badge>}
          />
          <SummaryCard
            label="Top memory now"
            value={memoryLeader ? formatBytes(memoryLeader.working_set) : "--"}
            valueClassName="text-base font-semibold sm:text-lg"
            accent={<Badge variant="info">Memory</Badge>}
          />
        </div>

        <Card className="space-y-0 overflow-hidden p-0">
          <div className="border-b border-border px-4 py-3 sm:px-5">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <div className="eyebrow">Process table</div>
                <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground">Live process list</h2>
                <p className="mt-1 text-sm leading-6 text-secondary">
                  Dense operator view for triage and guarded actions. Select a row only when you need socket and footprint detail.
                </p>
              </div>
              <div className="grid gap-2 sm:grid-cols-3 lg:w-auto">
                <div className="soft-panel">
                  <div className="metric-label">Focus</div>
                  <div className="mt-1 truncate text-sm font-semibold text-foreground">{topProcess ? topProcess.name : "No process"}</div>
                  <div className="mt-0.5 text-xs text-secondary">{topProcess ? `${topProcess.cpu_percent.toFixed(1)}% CPU` : "Waiting for data"}</div>
                </div>
                <div className="soft-panel">
                  <div className="metric-label">Selection</div>
                  <div className="mt-1 truncate text-sm font-semibold text-foreground">{detailProcess ? detailProcess.name : "Nothing selected"}</div>
                  <div className="mt-0.5 text-xs text-secondary">
                    {detailProcess ? `PID ${detailProcess.pid}` : "Click Details for ports and connections"}
                  </div>
                </div>
                <div className="soft-panel">
                  <div className="metric-label">Guardrails</div>
                  <div className="mt-1 text-sm font-semibold text-foreground">Critical + self protected</div>
                  <div className="mt-0.5 text-xs text-secondary">Unsafe actions stay blocked automatically.</div>
                </div>
              </div>
            </div>
          </div>

          {filteredProcesses.length === 0 ? (
            <div className="p-4 sm:p-5">
              <EmptyState icon={Boxes} title="No processes match" description="Try a different name or PID filter." />
            </div>
          ) : null}

          <div className="grid gap-4 p-4 md:hidden sm:p-5">
            {filteredProcesses.map((process) => (
              <ProcessCard
                key={process.pid}
                process={process}
                isPending={actionMutation.isPending}
                isSelf={process.pid === selfPID}
                onDetails={() => setDetailProcess(process)}
                onKill={() => setKillCandidate(process)}
                onResume={() => actionMutation.mutate({ pid: process.pid, action: "resume" })}
                onSuspend={() => actionMutation.mutate({ pid: process.pid, action: "suspend" })}
              />
            ))}
          </div>

          <div className={filteredProcesses.length === 0 ? "hidden" : "hidden md:block"} ref={tableRef}>
            <table className="dense-table min-w-full table-fixed text-left text-sm">
              <thead className="border-b border-border bg-background/55">
                <tr>
                  <th className="w-18 px-4 py-3 pr-3 sm:px-5">
                    <SortHeader label="PID" isActive={sortKey === "pid"} direction={sortDirection} onClick={() => updateSort("pid")} />
                  </th>
                  <th className="min-w-0 py-3 pr-3">
                    <SortHeader label="Name" isActive={sortKey === "name"} direction={sortDirection} onClick={() => updateSort("name")} />
                  </th>
                  <th className="w-18 py-3 pr-3">
                    <SortHeader label="CPU" isActive={sortKey === "cpu"} direction={sortDirection} onClick={() => updateSort("cpu")} />
                  </th>
                  <th className="w-24 py-3 pr-3">
                    <SortHeader label="Memory" isActive={sortKey === "memory"} direction={sortDirection} onClick={() => updateSort("memory")} />
                  </th>
                  <th className="w-16 py-3 pr-3">
                    <SortHeader label="Ports" isActive={sortKey === "connections"} direction={sortDirection} onClick={() => updateSort("connections")} />
                  </th>
                  <th className="w-20 py-3 pr-3">
                    <SortHeader label="Threads" isActive={sortKey === "threads"} direction={sortDirection} onClick={() => updateSort("threads")} />
                  </th>
                  <th className="w-22 py-3 pr-3">State</th>
                  <th className="w-40 py-3 pr-4 sm:pr-5">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredProcesses.map((process, index) => {
                  const isSelf = process.pid === selfPID;
                  const actionsDisabled = actionMutation.isPending || process.is_critical || isSelf;
                  const rowState = isSelf ? "WTM" : process.is_critical ? "Critical" : "Ready";
                  const isSelected = index === selectedIndex;

                  return (
                    <tr
                      key={process.pid}
                      className={
                        isSelected
                          ? "border-b border-border bg-accent text-accent-foreground transition-colors"
                          : detailProcess?.pid === process.pid
                          ? "border-b border-border bg-accent-muted/45 transition-colors"
                          : "border-b border-border transition-colors hover:bg-background/55"
                      }
                      onClick={() => setDetailProcess(process)}
                    >
                      <td className="px-4 py-2.5 pr-3 font-mono whitespace-nowrap sm:px-5">
                        <button
                          type="button"
                          className="font-mono text-secondary hover:text-accent transition-colors"
                          onClick={(e) => { e.stopPropagation(); navigate(`/processes?pid=${process.pid}`); }}
                          title={`Filter to PID ${process.pid}`}
                        >
                          {process.pid}
                        </button>
                      </td>
                      <td className="max-w-[12rem] py-2.5 pr-3 text-foreground">
                        <button
                          type="button"
                          className="block max-w-full truncate text-left font-medium hover:text-accent"
                          onClick={() => setDetailProcess(process)}
                          title={process.name}
                        >
                          {process.name}
                        </button>
                      </td>
                      <td className="py-2.5 pr-3 tabular-nums text-secondary whitespace-nowrap">{process.cpu_percent.toFixed(1)}%</td>
                      <td className="py-2.5 pr-3 tabular-nums text-secondary whitespace-nowrap">{formatBytes(process.working_set)}</td>
                      <td className="py-2.5 pr-3 tabular-nums text-secondary whitespace-nowrap">{process.connections}</td>
                      <td className="py-2.5 pr-3 tabular-nums text-secondary whitespace-nowrap">{process.thread_count}</td>
                      <td className="py-2.5 pr-3 text-xs font-semibold text-secondary whitespace-nowrap">{rowState}</td>
                      <td className="py-2.5 pr-4 sm:pr-5">
                        <div className="flex items-center gap-1.5">
                          <Button size="sm" variant="secondary" onClick={() => setDetailProcess(process)}>
                            Details
                          </Button>
                          <Button size="sm" variant="danger" disabled={actionsDisabled} onClick={() => setKillCandidate(process)}>
                            Kill
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>

      </div>

      {detailProcess && createPortal(
        <ProcessDetailModal
          process={detailProcess}
          isSelf={detailProcess.pid === selfPID}
          connections={processConnections}
          connectionsLoading={connectionsLoading}
          portSummary={portSummary}
          actionsDisabled={actionMutation.isPending || detailProcess.is_critical || detailProcess.pid === selfPID}
          onClose={() => setDetailProcess(null)}
          onKill={() => { setDetailProcess(null); setKillCandidate(detailProcess); }}
          onSuspend={() => actionMutation.mutate({ pid: detailProcess.pid, action: "suspend" })}
          onResume={() => actionMutation.mutate({ pid: detailProcess.pid, action: "resume" })}
        />,
        document.body,
      )}
      <ConfirmDialog
        open={killCandidate !== null}
        title={killCandidate ? `Kill ${killCandidate.name}?` : "Kill process?"}
        description={
          killCandidate
            ? `This will terminate ${killCandidate.name} (PID ${killCandidate.pid}). WTM protects critical processes and its own PID automatically.`
            : "This will terminate the selected process."
        }
        confirmLabel="Kill process"
        isPending={actionMutation.isPending}
        onOpenChange={(open) => {
          if (!open) {
            setKillCandidate(null);
          }
        }}
        onConfirm={() => {
          if (!killCandidate) {
            return;
          }
          actionMutation.mutate(
            { pid: killCandidate.pid, action: "kill" },
            {
              onSuccess: () => {
                setKillCandidate(null);
              },
            },
          );
        }}
      />
    </>
  );
}

interface ProcessCardProps {
  process: ProcessInfo;
  isPending: boolean;
  isSelf: boolean;
  onDetails: () => void;
  onKill: () => void;
  onResume: () => void;
  onSuspend: () => void;
}

function ProcessCard({ process, isPending, isSelf, onDetails, onKill, onResume, onSuspend }: ProcessCardProps) {
  const actionsDisabled = isPending || process.is_critical || isSelf;

  return (
    <Card className="space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-lg font-semibold tracking-tight text-foreground">{process.name}</div>
          <div className="mt-1 font-mono text-sm text-secondary">PID {process.pid}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          {process.is_critical ? <Badge variant="warning">Critical</Badge> : <Badge variant="neutral">Normal</Badge>}
          {isSelf ? <Badge variant="error">WTM</Badge> : null}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2.5 text-sm">
        <DetailTile label="CPU" value={`${process.cpu_percent.toFixed(1)}%`} />
        <DetailTile label="Memory" value={formatBytes(process.working_set)} />
        <DetailTile label="Ports" value={String(process.connections)} />
        <DetailTile label="Threads" value={String(process.thread_count)} />
      </div>
      <div className="soft-panel text-sm text-secondary">
        Ports shows how many live socket bindings this process owns. Open <span className="font-semibold text-foreground">Details</span> to see actual local ports and remote endpoints.
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button type="button" variant="ghost" onClick={onDetails}>
          <PlugZap className="mr-2 h-4 w-4" />
          Details
        </Button>
        <Button type="button" variant="danger" disabled={actionsDisabled} onClick={onKill}>
          Kill
        </Button>
        <Button type="button" variant="secondary" disabled={actionsDisabled} onClick={onSuspend}>
          Suspend
        </Button>
        <Button type="button" variant="ghost" disabled={isPending || isSelf} onClick={onResume}>
          Resume
        </Button>
      </div>
      {isSelf ? (
        <div className="flex items-center gap-2 rounded-lg border border-error bg-[color:var(--error-bg)] px-3 py-2.5 text-sm text-error">
          <ShieldBan className="h-4 w-4 shrink-0" />
          WTM protects its own process from terminate/suspend actions.
        </div>
      ) : null}
    </Card>
  );
}

interface PortUsageRowProps {
  binding: PortBinding;
}

function PortUsageRow({ binding }: PortUsageRowProps) {
  return (
    <div className="rounded-md border border-border bg-background px-3 py-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0 truncate text-sm font-semibold text-foreground">
          {binding.protocol.toUpperCase()} {binding.local_addr}:{binding.local_port}
        </div>
        <Badge variant={binding.state === "LISTEN" ? "success" : "info"}>{binding.state || "OPEN"}</Badge>
      </div>
      <div className="mt-1.5 truncate text-xs text-secondary">
        {binding.remote_addr || binding.remote_port > 0 ? `${binding.remote_addr}:${binding.remote_port}` : "No remote peer"}
      </div>
    </div>
  );
}

interface SortHeaderProps {
  label: string;
  isActive: boolean;
  direction: SortDirection;
  onClick: () => void;
}

function SortHeader({ label, isActive, direction, onClick }: SortHeaderProps) {
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 whitespace-nowrap font-semibold text-secondary transition-colors hover:text-foreground"
      onClick={onClick}
      aria-label={`Sort by ${label}`}
    >
      <span>{label}</span>
      {isActive ? (
        direction === "desc" ? <ArrowDown className="h-4 w-4" /> : <ArrowUp className="h-4 w-4" />
      ) : (
        <span className="text-xs text-muted">-</span>
      )}
    </button>
  );
}

function compareProcesses(left: ProcessInfo, right: ProcessInfo, sortKey: ProcessSortKey, direction: SortDirection) {
  let result = 0;
  switch (sortKey) {
    case "cpu":
      result = left.cpu_percent - right.cpu_percent;
      break;
    case "memory":
      result = left.working_set - right.working_set;
      break;
    case "name":
      result = left.name.localeCompare(right.name);
      break;
    case "threads":
      result = left.thread_count - right.thread_count;
      break;
    case "connections":
      result = left.connections - right.connections;
      break;
    case "pid":
      result = left.pid - right.pid;
      break;
  }
  return direction === "desc" ? result * -1 : result;
}

function summarizePorts(bindings: PortBinding[]) {
  let listeningPorts = 0;
  let remoteEndpoints = 0;
  let tcpRows = 0;
  let udpRows = 0;

  for (const binding of bindings) {
    if (binding.state === "LISTEN") {
      listeningPorts += 1;
    }
    if (binding.remote_addr || binding.remote_port > 0) {
      remoteEndpoints += 1;
    }
    if (binding.protocol.toLowerCase() === "tcp") {
      tcpRows += 1;
    }
    if (binding.protocol.toLowerCase() === "udp") {
      udpRows += 1;
    }
  }

  return { listeningPorts, remoteEndpoints, tcpRows, udpRows };
}

interface ProcessDetailModalProps {
  process: ProcessInfo;
  isSelf: boolean;
  connections: PortBinding[];
  connectionsLoading: boolean;
  portSummary: ReturnType<typeof summarizePorts>;
  actionsDisabled: boolean;
  onClose: () => void;
  onKill: () => void;
  onSuspend: () => void;
  onResume: () => void;
}

function ProcessDetailModal({
  process,
  isSelf,
  connections,
  connectionsLoading,
  portSummary,
  actionsDisabled,
  onClose,
  onKill,
  onSuspend,
  onResume,
}: ProcessDetailModalProps) {
  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", handler);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", handler);
    };
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-[300] flex items-center justify-center px-4 py-8" aria-modal="true">
      <button type="button" aria-label="Close" className="absolute inset-0 bg-[color:var(--overlay)] backdrop-blur-sm" onClick={onClose} />
      <div className="relative z-[301] w-full max-w-2xl rounded-3xl border border-border bg-surface shadow-lg max-h-[85vh] flex flex-col">
        {/* Header */}
        <div className="flex items-start justify-between gap-4 border-b border-border px-6 py-5">
          <div className="min-w-0">
            <div className="eyebrow">Process detail</div>
            <h2 className="mt-2 truncate text-xl font-semibold tracking-tight text-foreground" title={process.name}>{process.name}</h2>
            <p className="mt-1 text-sm text-secondary">PID {process.pid}</p>
          </div>
          <div className="flex items-center gap-3 shrink-0">
            <div className="flex flex-wrap gap-2">
              {process.is_critical && <Badge variant="warning">Critical</Badge>}
              {isSelf && <Badge variant="error">WTM self</Badge>}
            </div>
            <button type="button" aria-label="Close modal" className="rounded-lg border border-border p-1.5 text-secondary hover:bg-background-muted hover:text-foreground transition-colors" onClick={onClose}>
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-5 space-y-5">
          {/* Metrics */}
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <DetailTile label="CPU" value={`${process.cpu_percent.toFixed(1)}%`} />
            <DetailTile label="Memory" value={formatBytes(process.working_set)} />
            <DetailTile label="Threads" value={String(process.thread_count)} />
            <DetailTile label="Ports" value={String(process.connections)} />
          </div>

          {/* Actions */}
          <div className="space-y-3">
            <div className="text-sm font-semibold text-foreground">Actions</div>
            <div className="flex flex-wrap gap-2">
              <Button variant="danger" disabled={actionsDisabled} onClick={onKill}>
                Kill process
              </Button>
              <Button variant="secondary" disabled={actionsDisabled} onClick={onSuspend}>
                Suspend
              </Button>
              <Button variant="ghost" disabled={actionsDisabled} onClick={onResume}>
                Resume
              </Button>
            </div>
            {isSelf && (
              <div className="flex items-center gap-2 rounded-lg border border-error bg-[color:var(--error-bg)] px-3 py-2.5 text-sm text-error">
                <ShieldBan className="h-4 w-4 shrink-0" />
                WTM protects its own process from terminate and suspend actions.
              </div>
            )}
          </div>

          {/* Ports */}
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-foreground">Port usage</div>
                <div className="text-xs text-secondary">Live local ports and remote peers.</div>
              </div>
              <Badge variant="neutral">{connectionsLoading ? "..." : `${connections.length} rows`}</Badge>
            </div>
            {connectionsLoading ? (
              <p className="text-sm text-secondary">Loading...</p>
            ) : connections.length === 0 ? (
              <p className="text-sm text-secondary">No active ports in the current snapshot.</p>
            ) : (
              <div className="grid gap-2 sm:grid-cols-2">
                {connections.slice(0, 8).map((b) => (
                  <PortUsageRow key={`${b.protocol}-${b.local_port}-${b.remote_port}-${b.state}`} binding={b} />
                ))}
                {connections.length > 8 && (
                  <div className="text-xs text-secondary sm:col-span-2">+{connections.length - 8} more rows</div>
                )}
              </div>
            )}
          </div>

          {/* Connection summary */}
          {!connectionsLoading && connections.length > 0 && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <DetailTile label="Listening" value={String(portSummary.listeningPorts)} />
              <DetailTile label="Remote peers" value={String(portSummary.remoteEndpoints)} />
              <DetailTile label="TCP rows" value={String(portSummary.tcpRows)} />
              <DetailTile label="UDP rows" value={String(portSummary.udpRows)} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
