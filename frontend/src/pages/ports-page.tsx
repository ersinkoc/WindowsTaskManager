import { useMemo, useState } from "react";
import { ArrowDownAZ, ArrowUpAZ, Network, X } from "lucide-react";
import { useNavigate } from "react-router";
import { EmptyState } from "../components/shared/empty-state";
import { FilterChip } from "../components/shared/filter-chip";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { SearchInput } from "../components/shared/search-input";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { useSystemSnapshotQuery } from "../lib/api-client";
import type { PortBinding } from "../types/api";
import { cn } from "../lib/cn";

type PortSortKey = "process" | "pid" | "protocol" | "local" | "remote" | "state";
type SortDirection = "desc" | "asc";
type PortFilter = "all" | "tcp" | "udp" | "listening";

const WELL_KNOWN_PORTS: Record<number, { app: string; description: string }> = {
  20: { app: "FTP Data", description: "FTP active data channel" },
  21: { app: "FTP", description: "File transfer control" },
  22: { app: "SSH", description: "Secure shell" },
  25: { app: "SMTP", description: "Email sending" },
  53: { app: "DNS", description: "Name resolver" },
  80: { app: "HTTP", description: "Web server" },
  110: { app: "POP3", description: "Email retrieval" },
  143: { app: "IMAP", description: "Email retrieval" },
  443: { app: "HTTPS", description: "Encrypted web" },
  445: { app: "SMB", description: "Windows file sharing" },
  993: { app: "IMAPS", description: "Encrypted email" },
  995: { app: "POP3S", description: "Encrypted email" },
  1433: { app: "MSSQL", description: "Microsoft SQL Server" },
  1521: { app: "Oracle", description: "Oracle database" },
  3000: { app: "Node", description: "Dev server" },
  3306: { app: "MySQL", description: "MySQL / MariaDB" },
  3389: { app: "RDP", description: "Remote desktop" },
  5000: { app: "Flask", description: "Python dev server" },
  5173: { app: "Vite", description: "Vite / TS dev" },
  5432: { app: "PostgreSQL", description: "PostgreSQL database" },
  5672: { app: "RabbitMQ", description: "Message broker" },
  5900: { app: "VNC", description: "Virtual network computing" },
  6379: { app: "Redis", description: "In-memory data store" },
  7000: { app: "Redis", description: "Redis cluster" },
  8000: { app: "Django", description: "Django / Python dev" },
  8080: { app: "HTTP Alt", description: "Alt web server" },
  8443: { app: "HTTPS Alt", description: "Alt HTTPS" },
  8888: { app: "Jupyter", description: "Notebook / ML" },
  9000: { app: "PHP-FPM", description: "PHP FastCGI" },
  9090: { app: "Prometheus", description: "Metrics endpoint" },
  9200: { app: "Elasticsearch", description: "Search engine" },
  9300: { app: "Elasticsearch", description: "ES transport" },
  27017: { app: "MongoDB", description: "MongoDB database" },
  27018: { app: "MongoDB", description: "MongoDB shard" },
};

const DEV_PORTS = new Set([3000, 5000, 5173, 8000, 8888, 9090]);

const sortOptions: Array<{ label: string; value: PortSortKey }> = [
  { label: "Process", value: "process" },
  { label: "PID", value: "pid" },
  { label: "Protocol", value: "protocol" },
  { label: "Local port", value: "local" },
  { label: "Remote peer", value: "remote" },
  { label: "State", value: "state" },
];

function getKnownPort(local_port: number) {
  return WELL_KNOWN_PORTS[local_port] ?? null;
}

function portLabel(binding: PortBinding): string {
  const info = getKnownPort(binding.local_port);
  if (info) return info.description;
  if (binding.state === "LISTEN") return "Listening";
  if (binding.protocol.toLowerCase() === "udp") return "Datagram";
  if (binding.remote_addr || binding.remote_port > 0) return "Active connection";
  return "Open socket";
}

export function PortsPage() {
  const navigate = useNavigate();
  const { data, isLoading } = useSystemSnapshotQuery();
  const [searchValue, setSearchValue] = useState("");
  const [selectedBinding, setSelectedBinding] = useState<PortBinding | null>(null);
  const [sortKey, setSortKey] = useState<PortSortKey>("local");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");
  const [filter, setFilter] = useState<PortFilter>("all");
  const debouncedSearch = useDebouncedValue(searchValue, 300);
  const bindings = data?.port_bindings ?? [];

  const filteredBindings = useMemo(() => {
    const needle = debouncedSearch.trim().toLowerCase();
    const base = bindings.filter((b) => {
      if (filter === "tcp" && b.protocol.toLowerCase() !== "tcp") return false;
      if (filter === "udp" && b.protocol.toLowerCase() !== "udp") return false;
      if (filter === "listening" && b.state !== "LISTEN") return false;
      if (!needle) return true;
      return (
        b.protocol.toLowerCase().includes(needle) ||
        b.process.toLowerCase().includes(needle) ||
        b.label.toLowerCase().includes(needle) ||
        b.local_addr.toLowerCase().includes(needle) ||
        b.remote_addr.toLowerCase().includes(needle) ||
        String(b.pid).includes(needle) ||
        String(b.local_port).includes(needle) ||
        String(b.remote_port).includes(needle)
      );
    });
    return [...base].sort((l, r) => compareBindings(l, r, sortKey, sortDirection));
  }, [bindings, debouncedSearch, filter, sortDirection, sortKey]);

  const listeningCount = bindings.filter((b) => b.state === "LISTEN").length;
  const remotePeerCount = bindings.filter((b) => b.remote_addr || b.remote_port > 0).length;
  const uniquePIDCount = new Set(bindings.map((b) => b.pid)).size;

  const devBindings = filteredBindings.filter((b) => DEV_PORTS.has(b.local_port));

  const usedWellKnownPorts = useMemo(() => {
    const ports = new Set<number>();
    for (const b of filteredBindings) {
      if (WELL_KNOWN_PORTS[b.local_port]) ports.add(b.local_port);
    }
    return [...ports].sort((a, b) => a - b);
  }, [filteredBindings]);

  const nodeServerBindings = useMemo(
    () =>
      filteredBindings.filter(
        (b) =>
          (b.process === "node.exe" || b.process === "node" || b.process.endsWith("\\node.exe")) &&
          DEV_PORTS.has(b.local_port)
      ),
    [filteredBindings]
  );

  const nodeServersByPort = useMemo(() => {
    const map = new Map<number, PortBinding[]>();
    for (const b of nodeServerBindings) {
      const list = map.get(b.local_port) ?? [];
      list.push(b);
      map.set(b.local_port, list);
    }
    return [...map.entries()].sort((a, b) => a[0] - b[0]);
  }, [nodeServerBindings]);

  // Pre-compute per-port binding lists to avoid repeated .filter() calls
  const bindingsByPort = useMemo(() => {
    const map = new Map<number, PortBinding[]>();
    for (const b of filteredBindings) {
      const list = map.get(b.local_port) ?? [];
      list.push(b);
      map.set(b.local_port, list);
    }
    return map;
  }, [filteredBindings]);

  if (isLoading) return <PageSkeleton />;
  if (bindings.length === 0) {
    return <EmptyState icon={Network} title="No port bindings yet" description="Port monitor data will appear here when the collector publishes it." />;
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Ports"
        description="Which ports are occupied, what runs on them, and which process owns each one."
        eyebrow="Network ownership"
        icon={Network}
        meta={
          <>
            <Badge variant="info">{bindings.length}</Badge>
            <Badge variant="success">{listeningCount} LISTEN</Badge>
            <Badge variant="warning">{remotePeerCount} active</Badge>
          </>
        }
        actions={
          <>
            <SearchInput
              ariaLabel="Search ports"
              placeholder="Process, PID, address, or port"
              value={searchValue}
              widthClassName="sm:w-72"
              onChange={setSearchValue}
            />
            <select
              aria-label="Sort by"
              className="form-control min-h-9 w-auto px-2 text-xs"
              value={sortKey}
              onChange={(e) => setSortKey(e.target.value as PortSortKey)}
            >
              {sortOptions.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <Button type="button" variant="secondary" onClick={() => setSortDirection((c) => (c === "desc" ? "asc" : "desc"))}>
              {sortDirection === "desc" ? <ArrowDownAZ className="h-4 w-4" /> : <ArrowUpAZ className="h-4 w-4" />}
            </Button>
          </>
        }
      />

      {/* Summary strip */}
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="hero-panel flex items-center gap-4 p-3 sm:p-3.5">
          <div className="text-2xl font-bold tabular-nums text-foreground">{bindings.length}</div>
          <div>
            <div className="eyebrow">Bindings</div>
            <div className="mt-0.5 text-xs text-secondary">TCP + UDP</div>
          </div>
        </div>
        <div className="hero-panel flex items-center gap-4 p-3 sm:p-3.5">
          <div className="text-2xl font-bold tabular-nums text-success">{listeningCount}</div>
          <div>
            <div className="eyebrow">Listening</div>
            <div className="mt-0.5 text-xs text-secondary">Accepting inbound</div>
          </div>
        </div>
        <div className="hero-panel flex items-center gap-4 p-3 sm:p-3.5">
          <div className="text-2xl font-bold tabular-nums text-warning">{uniquePIDCount}</div>
          <div>
            <div className="eyebrow">Processes</div>
            <div className="mt-0.5 text-xs text-secondary">{remotePeerCount} remote peers</div>
          </div>
        </div>
      </div>

      {/* Node.js servers */}
      {nodeServersByPort.length > 0 && (
        <Card className="p-4 sm:p-5">
          <div className="mb-3 flex items-center gap-2">
            <span className="inline-flex h-6 items-center gap-1.5 rounded-md border border-[color:var(--accent)]/30 bg-[color:var(--accent)]/10 px-2.5 py-0.5 text-[0.65rem] font-semibold text-[color:var(--accent)]">
              Node.js
            </span>
            <span className="text-xs text-secondary">{nodeServersByPort.length} server instance{nodeServersByPort.length !== 1 ? "s" : ""}</span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {nodeServersByPort.map(([port, ports]) => (
              <PortCell key={port} port={port} bindings={ports} onDetails={() => setSelectedBinding(ports[0]!)} />
            ))}
          </div>
        </Card>
      )}

      {/* Dev ports */}
      {devBindings.length > 0 && (
        <Card className="p-4 sm:p-5">
          <div className="mb-3 flex items-center gap-2">
            <span className="inline-flex h-6 items-center gap-1.5 rounded-md border border-border bg-surface px-2.5 py-0.5 text-[0.65rem] font-semibold text-secondary">
              Dev Ports
            </span>
            <span className="text-xs text-secondary">Local dev servers — non-standard, per-project</span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {devBindings.map((b) => (
              <PortCell key={bindingKey(b)} port={b.local_port} bindings={[b]} onDetails={() => setSelectedBinding(b)} />
            ))}
          </div>
        </Card>
      )}

      {/* Known ports */}
      {usedWellKnownPorts.length > 0 && (
        <Card className="p-4 sm:p-5">
          <div className="mb-3 flex items-center gap-2">
            <span className="inline-flex h-6 items-center gap-1.5 rounded-md border border-border bg-surface px-2.5 py-0.5 text-[0.65rem] font-semibold text-secondary">
              Known
            </span>
            <span className="text-xs text-secondary">Registered ports — matched to application</span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {usedWellKnownPorts.map((port) => {
              const portBindings = bindingsByPort.get(port) ?? [];
              const info = WELL_KNOWN_PORTS[port]!;
              return (
                <PortCell
                  key={port}
                  port={port}
                  app={info.app}
                  description={info.description}
                  bindings={portBindings}
                  onDetails={() => setSelectedBinding(portBindings[0]!)}
                />
              );
            })}
          </div>
        </Card>
      )}

      {/* All bindings table */}
      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:px-5 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <div className="eyebrow">All bindings</div>
            <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground">
              {filteredBindings.length === bindings.length
                ? "Full port list"
                : `${filteredBindings.length} of ${bindings.length} shown`}
            </h2>
          </div>
          <div className="flex flex-wrap gap-1.5">
            <FilterChip active={filter === "all"} label="All" onClick={() => setFilter("all")} />
            <FilterChip active={filter === "tcp"} label="TCP" onClick={() => setFilter("tcp")} />
            <FilterChip active={filter === "udp"} label="UDP" onClick={() => setFilter("udp")} />
            <FilterChip active={filter === "listening"} label="Listening" onClick={() => setFilter("listening")} />
          </div>
        </div>

        <div className="grid gap-3 p-4 md:hidden sm:p-5">
          {filteredBindings.map((b) => (
            <PortCard key={bindingKey(b)} binding={b} onDetails={() => setSelectedBinding(b)} />
          ))}
        </div>

        <div className={filteredBindings.length === 0 ? "hidden" : "hidden md:block"}>
          <table className="dense-table min-w-full table-fixed text-left text-sm">
            <thead className="border-b border-border bg-background/55">
              <tr>
                <th className="w-14 px-4 py-3 pr-2 sm:px-5">Proto</th>
                <th className="w-28 py-3 pr-2">Local</th>
                <th className="w-28 py-3 pr-2">Remote</th>
                <th className="w-16 py-3 pr-2">State</th>
                <th className="w-14 py-3 pr-2">PID</th>
                <th className="py-3 pr-2">Process</th>
                <th className="w-14 py-3 pr-4 sm:pr-5"></th>
              </tr>
            </thead>
            <tbody>
              {filteredBindings.map((b) => (
                <tr
                  key={bindingKey(b)}
                  className="border-b border-border transition-colors hover:bg-background/55"
                >
                  <td className="px-4 py-2.5 pr-2 sm:px-5">
                    <span className="inline-flex h-6 items-center gap-1 rounded border border-border bg-background-muted/70 px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase text-secondary">
                      {b.protocol}
                    </span>
                  </td>
                  <td className="py-2.5 pr-2 font-mono text-sm text-foreground">{b.local_addr}:{b.local_port}</td>
                  <td className="py-2.5 pr-2 font-mono text-sm text-secondary">{formatRemote(b)}</td>
                  <td className="py-2.5 pr-2">
                    <span className={cn(
                      "inline-flex h-6 items-center gap-1 rounded border px-1.5 py-0.5 text-[0.65rem] font-semibold",
                      b.state === "LISTEN"
                        ? "border-[color:var(--success)]/30 bg-[color:var(--success)]/10 text-success"
                        : "border-border bg-background-muted/70 text-secondary"
                    )}>
                      {b.state || "OPEN"}
                    </span>
                  </td>
                  <td className="py-2.5 pr-2 font-mono text-sm text-secondary">
                    <button
                      type="button"
                      className="hover:text-accent text-secondary transition-colors"
                      onClick={() => navigate(`/processes?pid=${b.pid}`)}
                    >
                      {b.pid}
                    </button>
                  </td>
                  <td className="py-2.5 pr-2">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-foreground">{b.process || b.label || "—"}</div>
                      {getKnownPort(b.local_port) && (
                        <div className="truncate text-[0.72rem] text-secondary">
                          {getKnownPort(b.local_port)!.app}
                        </div>
                      )}
                    </div>
                  </td>
                  <td className="py-2.5 pr-4 sm:pr-5">
                    <Button size="sm" variant="ghost" onClick={() => setSelectedBinding(b)}>Inspect</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Inspector drawer */}
      {selectedBinding ? (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setSelectedBinding(null)} />
          <div className="fixed right-0 top-0 z-50 flex h-full w-80 flex-col border-l border-border bg-background shadow-xl">
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <div className="min-w-0 pr-3">
                <div className="eyebrow">Inspector</div>
                <h2 className="truncate text-base font-semibold tracking-tight text-foreground">
                  {selectedBinding.process || selectedBinding.label || "Unknown"}
                </h2>
              </div>
              <Button type="button" variant="ghost" size="sm" onClick={() => setSelectedBinding(null)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex flex-wrap gap-2 border-b border-border bg-surface px-4 py-3">
              <Badge variant={selectedBinding.state === "LISTEN" ? "success" : "info"}>{selectedBinding.state || "OPEN"}</Badge>
              <Badge variant="neutral">{selectedBinding.protocol.toUpperCase()}</Badge>
              <Badge variant="neutral">PID {selectedBinding.pid}</Badge>
              {getKnownPort(selectedBinding.local_port) && (
                <Badge variant="neutral">{getKnownPort(selectedBinding.local_port)!.app}</Badge>
              )}
            </div>
            <div className="flex-1 overflow-y-auto p-4">
              <div className="space-y-3">
                <DetailTile label="Process" value={selectedBinding.process || selectedBinding.label || "Unknown"} />
                {getKnownPort(selectedBinding.local_port) && (
                  <DetailTile label="App" value={getKnownPort(selectedBinding.local_port)!.app} />
                )}
                <DetailTile label="Local" value={`${selectedBinding.local_addr}:${selectedBinding.local_port}`} />
                <DetailTile label="Remote" value={formatRemote(selectedBinding)} />
                <DetailTile label="Meaning" value={portLabel(selectedBinding)} valueClassName="whitespace-normal" />
              </div>
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}

function DetailTile({ label, value, valueClassName = "" }: { label: string; value: string; valueClassName?: string }) {
  return (
    <div className="soft-panel min-w-0">
      <div className="metric-label">{label}</div>
      <div className={cn("mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm font-semibold text-foreground", valueClassName)}>{value}</div>
    </div>
  );
}

interface PortCellProps {
  port: number;
  app?: string;
  description?: string;
  bindings: PortBinding[];
  onDetails: () => void;
}

function PortCell({ port, app, description, bindings, onDetails }: PortCellProps) {
  const known = getKnownPort(port);
  const displayApp = app ?? known?.app;
  const displayDesc = description ?? known?.description;

  return (
    <button type="button" onClick={onDetails} className="soft-panel text-left w-full">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm font-bold text-foreground">{port}</span>
            {displayApp && <span className="text-xs text-secondary">{displayApp}</span>}
          </div>
          {displayDesc && <div className="mt-0.5 text-[0.72rem] text-secondary">{displayDesc}</div>}
          <div className="mt-1.5 flex flex-wrap gap-1">
            {bindings.map((b) => (
              <span key={bindingKey(b)} className="inline-flex h-5 items-center gap-1 rounded border border-border bg-background-muted/70 px-1.5 py-0.5 text-[0.62rem] font-medium text-secondary">
                {b.protocol.toUpperCase()}
              </span>
            ))}
          </div>
        </div>
        <span className="shrink-0 font-mono text-[0.7rem] font-semibold text-secondary">{bindings.length}x</span>
      </div>
    </button>
  );
}

interface PortCardProps {
  binding: PortBinding;
  onDetails: () => void;
}

function PortCard({ binding, onDetails }: PortCardProps) {
  const known = getKnownPort(binding.local_port);
  return (
    <Card className="space-y-2">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-base font-bold text-foreground">{binding.local_port}</span>
            {known && <span className="text-xs text-secondary">{known.app}</span>}
          </div>
          <div className="mt-1 truncate text-sm font-medium text-foreground">{binding.process || binding.label || "—"}</div>
          <div className="mt-0.5 text-xs text-secondary">PID {binding.pid}</div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <span className={cn(
            "inline-flex h-6 items-center gap-1 rounded border px-1.5 py-0.5 text-[0.65rem] font-semibold",
            binding.state === "LISTEN"
              ? "border-[color:var(--success)]/30 bg-[color:var(--success)]/10 text-success"
              : "border-border bg-background-muted/70 text-secondary"
          )}>
            {binding.state || "OPEN"}
          </span>
          <span className="inline-flex h-6 items-center gap-1 rounded border border-border bg-background-muted/70 px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase text-secondary">
            {binding.protocol}
          </span>
        </div>
      </div>
      <div className="text-xs text-secondary">{binding.local_addr}:{binding.local_port} → {formatRemote(binding)}</div>
      <Button type="button" variant="ghost" size="sm" onClick={onDetails}>Inspect</Button>
    </Card>
  );
}

function bindingKey(binding: PortBinding) {
  return `${binding.protocol}-${binding.pid}-${binding.local_addr}-${binding.local_port}-${binding.remote_addr}-${binding.remote_port}-${binding.state}`;
}

function formatRemote(binding: PortBinding) {
  if (!binding.remote_addr && binding.remote_port === 0) return "—";
  return `${binding.remote_addr}:${binding.remote_port}`;
}

function compareBindings(left: PortBinding, right: PortBinding, sortKey: PortSortKey, direction: SortDirection) {
  let result = 0;
  switch (sortKey) {
    case "process": result = (left.process || left.label).localeCompare(right.process || right.label); break;
    case "pid": result = left.pid - right.pid; break;
    case "protocol": result = left.protocol.localeCompare(right.protocol); break;
    case "local": result = left.local_port - right.local_port; break;
    case "remote": result = left.remote_port - right.remote_port; break;
    case "state": result = (left.state || "").localeCompare(right.state || ""); break;
  }
  return direction === "desc" ? result * -1 : result;
}
