import { useMemo, useState } from "react";
import { ArrowDownAZ, ArrowUpAZ, Network, X } from "lucide-react";
import { useNavigate } from "react-router";
import { DetailTile, SummaryCard } from "../components/shared/detail-tile";
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

type PortSortKey = "process" | "pid" | "protocol" | "local" | "remote" | "state";
type SortDirection = "desc" | "asc";
type PortFilter = "all" | "tcp" | "udp" | "listening";

const sortOptions: Array<{ label: string; value: PortSortKey }> = [
  { label: "Process", value: "process" },
  { label: "PID", value: "pid" },
  { label: "Protocol", value: "protocol" },
  { label: "Local port", value: "local" },
  { label: "Remote peer", value: "remote" },
  { label: "State", value: "state" },
];

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
    const base = bindings.filter((binding) => {
      if (filter === "tcp" && binding.protocol.toLowerCase() !== "tcp") {
        return false;
      }
      if (filter === "udp" && binding.protocol.toLowerCase() !== "udp") {
        return false;
      }
      if (filter === "listening" && binding.state !== "LISTEN") {
        return false;
      }
      if (!needle) {
        return true;
      }
      return (
        binding.protocol.toLowerCase().includes(needle) ||
        binding.process.toLowerCase().includes(needle) ||
        binding.label.toLowerCase().includes(needle) ||
        binding.local_addr.toLowerCase().includes(needle) ||
        binding.remote_addr.toLowerCase().includes(needle) ||
        String(binding.pid).includes(needle) ||
        String(binding.local_port).includes(needle) ||
        String(binding.remote_port).includes(needle)
      );
    });

    const sorted = [...base];
    sorted.sort((left, right) => compareBindings(left, right, sortKey, sortDirection));
    return sorted;
  }, [bindings, debouncedSearch, filter, sortDirection, sortKey]);

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (bindings.length === 0) {
    return <EmptyState icon={Network} title="No port bindings yet" description="Port monitor data will appear here when the collector publishes it." />;
  }

  const listeningCount = bindings.filter((binding) => binding.state === "LISTEN").length;
  const remotePeerCount = bindings.filter((binding) => hasRemotePeer(binding)).length;
  const uniquePIDCount = new Set(bindings.map((binding) => binding.pid)).size;
  const topOwners = topBindingOwners(filteredBindings);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Ports"
        description="Inspect endpoint ownership, distinguish listeners from live peers, and narrow quickly by process or address."
        eyebrow="Network ownership"
        icon={Network}
        meta={
          <>
            <Badge variant="info">{bindings.length} bindings</Badge>
            <Badge variant="success">{listeningCount} listeners</Badge>
            <Badge variant="warning">{remotePeerCount} remote peers</Badge>
          </>
        }
        actions={
          <>
            <SearchInput
              ariaLabel="Search ports by process, PID, address, or port"
              placeholder="Search process, PID, address, or port"
              value={searchValue}
              widthClassName="sm:w-80"
              onChange={setSearchValue}
            />
            <label className="flex min-h-9 items-center gap-2 rounded-lg border border-border bg-background px-3 text-sm text-secondary">
              <span>Sort</span>
              <select
                aria-label="Sort port bindings"
                className="bg-transparent text-foreground"
                value={sortKey}
                onChange={(event) => setSortKey(event.target.value as PortSortKey)}
              >
                {sortOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <Button type="button" variant="secondary" onClick={() => setSortDirection((current) => (current === "desc" ? "asc" : "desc"))}>
              {sortDirection === "desc" ? <ArrowDownAZ className="mr-2 h-4 w-4" /> : <ArrowUpAZ className="mr-2 h-4 w-4" />}
              {sortDirection === "desc" ? "High first" : "Low first"}
            </Button>
          </>
        }
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <SummaryCard label="Visible bindings" value={String(filteredBindings.length)} accent={<Badge variant="info">Filtered</Badge>} />
        <SummaryCard label="Listening ports" value={String(listeningCount)} accent={<Badge variant="success">LISTEN</Badge>} />
        <SummaryCard label="Processes using ports" value={String(uniquePIDCount)} accent={<Badge variant="warning">{remotePeerCount} remote peers</Badge>} />
      </div>

      <Card className="space-y-0 overflow-hidden p-0">
        <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:px-5 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <div className="eyebrow">Port table</div>
            <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground">Live bindings</h2>
            <p className="mt-1 text-sm leading-6 text-secondary">Dense network ownership view with filters, sorting, and quick row inspection.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <FilterChip active={filter === "all"} label="All" onClick={() => setFilter("all")} />
            <FilterChip active={filter === "tcp"} label="TCP" onClick={() => setFilter("tcp")} />
            <FilterChip active={filter === "udp"} label="UDP" onClick={() => setFilter("udp")} />
            <FilterChip active={filter === "listening"} label="Listening" onClick={() => setFilter("listening")} />
          </div>
        </div>

        {filteredBindings.length === 0 ? (
          <div className="p-4 sm:p-5">
            <EmptyState icon={Network} title="No bindings match" description="Try a different process, PID, protocol, state, address, or port filter." />
          </div>
        ) : null}

        <div className="grid gap-4 p-4 md:hidden sm:p-5">
          {filteredBindings.map((binding) => (
            <PortCard
              key={bindingKey(binding)}
              binding={binding}
              selected={selectedBinding !== null && bindingKey(selectedBinding) === bindingKey(binding)}
              onDetails={() => setSelectedBinding(binding)}
            />
          ))}
        </div>

        <div className={filteredBindings.length === 0 ? "hidden" : "hidden md:block"}>
          <table className="dense-table min-w-full table-fixed text-left text-sm">
            <thead className="border-b border-border bg-background/55">
              <tr>
                <th className="w-18 px-4 py-3 pr-3 sm:px-5">Protocol</th>
                <th className="w-32 py-3 pr-3">Local</th>
                <th className="w-32 py-3 pr-3">Remote</th>
                <th className="w-18 py-3 pr-3">State</th>
                <th className="w-18 py-3 pr-3">PID</th>
                <th className="py-3 pr-3">Process</th>
                <th className="w-18 py-3 pr-4 sm:pr-5">Action</th>
              </tr>
            </thead>
            <tbody>
              {filteredBindings.map((binding) => (
                <tr
                  key={bindingKey(binding)}
                  className={
                    selectedBinding !== null && bindingKey(selectedBinding) === bindingKey(binding)
                      ? "border-b border-border bg-accent-muted/45"
                      : "border-b border-border transition-colors hover:bg-background/55"
                  }
                >
                  <td className="px-4 py-2.5 pr-3 uppercase text-secondary whitespace-nowrap sm:px-5">{binding.protocol}</td>
                  <td className="py-2.5 pr-3 text-foreground whitespace-nowrap">{binding.local_addr}:{binding.local_port}</td>
                  <td className="py-2.5 pr-3 text-secondary whitespace-nowrap">{formatRemote(binding)}</td>
                  <td className="py-2.5 pr-3 text-secondary whitespace-nowrap">{binding.state || "OPEN"}</td>
                  <td className="py-2.5 pr-3 font-mono whitespace-nowrap">
                    <button
                      type="button"
                      className="hover:text-accent text-secondary transition-colors"
                      onClick={() => navigate(`/processes?pid=${binding.pid}`)}
                      title={`View PID ${binding.pid} in processes`}
                    >
                      {binding.pid}
                    </button>
                  </td>
                  <td className="py-2.5 pr-3 text-secondary">
                    <div className="truncate font-medium text-foreground">{binding.process || binding.label || "Unknown process"}</div>
                  </td>
                  <td className="py-2.5 pr-4 sm:pr-5">
                    <Button size="sm" variant="ghost" onClick={() => setSelectedBinding(binding)}>
                      Details
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="grid gap-2 border-t border-border p-4 sm:grid-cols-3 sm:p-5 xl:grid-cols-4">
          {topOwners.map((owner) => (
            <div key={`${owner.process}-${owner.pid}`} className="soft-panel">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-foreground">{owner.process}</div>
                  <div className="mt-0.5 text-xs text-secondary">PID {owner.pid}</div>
                </div>
                <Badge variant="neutral">{owner.count}</Badge>
              </div>
            </div>
          ))}
        </div>
      </Card>

      {selectedBinding ? (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setSelectedBinding(null)} />
          <div className="fixed right-0 top-0 z-50 flex h-full w-96 flex-col border-l border-border bg-background shadow-xl">
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <div className="min-w-0">
                <div className="eyebrow">Inspector</div>
                <h2 className="truncate text-lg font-semibold tracking-tight text-foreground">
                  {selectedBinding.process || selectedBinding.label || "Unknown process"}
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
            </div>
            <div className="flex-1 overflow-y-auto p-4">
              <div className="space-y-3">
                <DetailTile label="Process" value={selectedBinding.process || selectedBinding.label || "Unknown"} />
                <DetailTile label="Local endpoint" value={`${selectedBinding.local_addr}:${selectedBinding.local_port}`} />
                <DetailTile label="Remote peer" value={formatRemote(selectedBinding)} />
                <DetailTile label="Usage meaning" value={portUsageMeaning(selectedBinding)} valueClassName="whitespace-normal leading-6" />
              </div>
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}

interface PortCardProps {
  binding: PortBinding;
  selected: boolean;
  onDetails: () => void;
}

function PortCard({ binding, selected, onDetails }: PortCardProps) {
  return (
    <Card className={selected ? "space-y-3 bg-background" : "space-y-3"}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-lg font-semibold tracking-tight text-foreground">{binding.process || binding.label || "Unknown process"}</div>
          <div className="mt-1 font-mono text-sm text-secondary">PID {binding.pid}</div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge variant={binding.state === "LISTEN" ? "success" : "info"}>{binding.state || "OPEN"}</Badge>
          <Badge variant="neutral">{binding.protocol.toUpperCase()}</Badge>
        </div>
      </div>
      <div className="grid gap-3 text-sm">
        <DetailTile label="Local" value={`${binding.local_addr}:${binding.local_port}`} valueClassName="break-all" />
        <DetailTile label="Remote" value={formatRemote(binding)} valueClassName="break-all" />
        <DetailTile label="Usage" value={portUsageMeaning(binding)} valueClassName="leading-snug" />
      </div>
      <Button type="button" variant="ghost" onClick={onDetails}>
        Details
      </Button>
    </Card>
  );
}

function bindingKey(binding: PortBinding) {
  return `${binding.protocol}-${binding.pid}-${binding.local_addr}-${binding.local_port}-${binding.remote_addr}-${binding.remote_port}-${binding.state}`;
}

function formatRemote(binding: PortBinding) {
  if (!hasRemotePeer(binding)) {
    return "No remote peer";
  }
  return `${binding.remote_addr}:${binding.remote_port}`;
}

function hasRemotePeer(binding: PortBinding) {
  return Boolean(binding.remote_addr) || binding.remote_port > 0;
}

function portUsageMeaning(binding: PortBinding) {
  if (binding.state === "LISTEN") {
    return "Waiting for inbound connections on the local port.";
  }
  if (binding.protocol.toLowerCase() === "udp") {
    return "Datagram socket used for connectionless traffic.";
  }
  if (hasRemotePeer(binding)) {
    return "Active connection between a local socket and a remote peer.";
  }
  return "Socket exists but no remote peer is currently attached.";
}

function compareBindings(left: PortBinding, right: PortBinding, sortKey: PortSortKey, direction: SortDirection) {
  let result = 0;
  switch (sortKey) {
    case "process":
      result = (left.process || left.label).localeCompare(right.process || right.label);
      break;
    case "pid":
      result = left.pid - right.pid;
      break;
    case "protocol":
      result = left.protocol.localeCompare(right.protocol);
      break;
    case "local":
      result = left.local_port - right.local_port;
      break;
    case "remote":
      result = left.remote_port - right.remote_port;
      break;
    case "state":
      result = (left.state || "").localeCompare(right.state || "");
      break;
  }
  return direction === "desc" ? result * -1 : result;
}

function topBindingOwners(bindings: PortBinding[]) {
  const counts = new Map<string, { process: string; pid: number; count: number }>();
  for (const binding of bindings) {
    const key = `${binding.process || binding.label || "Unknown"}-${binding.pid}`;
    const current = counts.get(key);
    if (current) {
      current.count += 1;
      continue;
    }
    counts.set(key, {
      process: binding.process || binding.label || "Unknown",
      pid: binding.pid,
      count: 1,
    });
  }
  return [...counts.values()].sort((left, right) => right.count - left.count).slice(0, 4);
}
