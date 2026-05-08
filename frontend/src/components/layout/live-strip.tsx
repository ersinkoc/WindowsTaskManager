import { useMemo } from "react";
import { useLocation } from "react-router";
import { useLocalStorage } from "../../hooks/use-local-storage";
import { useSystemSnapshotQuery } from "../../lib/api-client";
import { formatBytes, formatRate } from "../../lib/format";
import { meterBarClassName } from "../../lib/meters";
import type { InterfaceInfo } from "../../types/api";

function pickPrimaryInterface(interfaces: InterfaceInfo[]) {
  let best: InterfaceInfo | null = null;
  let bestScore = -1;
  for (const iface of interfaces) {
    const score = iface.in_bps + iface.out_bps + iface.in_pps + iface.out_pps;
    if (score > bestScore) {
      best = iface;
      bestScore = score;
    }
  }
  return best;
}

export function LiveStrip() {
  const location = useLocation();
  const { data } = useSystemSnapshotQuery();
  const [selectedInterface, setSelectedInterface] = useLocalStorage<string>("wtm-network-interface", "auto");
  const hidden = location.pathname === "/" || location.pathname === "/about";

  const activeInterface = useMemo(() => {
    if (!data) {
      return null;
    }
    if (selectedInterface !== "auto") {
      const manual = data.network.interfaces.find((iface) => iface.name === selectedInterface);
      if (manual) {
        return manual;
      }
    }
    return pickPrimaryInterface(data.network.interfaces);
  }, [data, selectedInterface]);

  const networkLabel = useMemo(() => {
    if (!data) {
      return "--";
    }
    if (activeInterface) {
      return `${formatRate(activeInterface.in_bps)} down / ${formatRate(activeInterface.out_bps)} up`;
    }
    return `${formatRate(data.network.total_down_bps)} down / ${formatRate(data.network.total_up_bps)} up`;
  }, [activeInterface, data]);

  if (hidden || !data) {
    return null;
  }

  return (
    <section className="page-padding mt-3">
      <div className="shell-panel overflow-hidden">
        <div className="grid gap-px bg-border md:grid-cols-2 2xl:grid-cols-[repeat(5,minmax(0,1fr))_14rem]">
          <Pill label="CPU" value={`${data.cpu.total_percent.toFixed(1)}%`} detail={`${data.cpu.num_logical} logical cores`} meter={data.cpu.total_percent} />
          <Pill
            label="Memory"
            value={`${data.memory.used_percent.toFixed(1)}%`}
            detail={`${formatBytes(data.memory.used_phys)} of ${formatBytes(data.memory.total_phys)}`}
            meter={data.memory.used_percent}
          />
          <Pill
            label="GPU"
            value={data.gpu.available ? `${data.gpu.utilization.toFixed(0)}%` : "n/a"}
            detail={data.gpu.available ? data.gpu.name || "Graphics processor" : "GPU unavailable"}
            meter={data.gpu.available ? data.gpu.utilization : undefined}
          />
          <Pill label="Network" value={networkLabel} detail={activeInterface?.name ?? "All interfaces"} />
          <Pill label="Processes" value={String(data.processes.length)} detail="Tracked right now" />
          <label className="flex min-h-[4.4rem] min-w-0 flex-col justify-center gap-1.5 bg-surface px-3 py-2.5 text-xs text-secondary">
            <span className="font-mono uppercase tracking-[0.18em]">Interface</span>
          <select
            aria-label="Select the network interface shown in the live strip"
            className="h-8 rounded-md border border-border bg-background px-2.5 py-1.5 font-mono text-sm text-foreground focus-visible:ring-2 focus-visible:ring-[var(--ring)]"
            value={selectedInterface}
            onChange={(event) => setSelectedInterface(event.target.value)}
          >
            <option value="auto">Auto</option>
            {data.network.interfaces.map((iface) => (
              <option key={iface.name} value={iface.name}>
                {iface.name}
              </option>
            ))}
          </select>
          </label>
        </div>
      </div>
    </section>
  );
}

interface PillProps {
  label: string;
  value: string;
  detail: string;
  meter?: number;
}

function Pill({ label, value, detail, meter }: PillProps) {
  return (
    <div className="flex min-h-[4.4rem] min-w-0 flex-col justify-center bg-surface px-3 py-2.5">
      <span className="text-[0.67rem] font-medium uppercase tracking-[0.18em] text-secondary">{label}</span>
      <span className="mt-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm font-semibold tracking-tight tabular-nums text-foreground lg:text-[0.98rem]">{value}</span>
      <span className="mt-0.5 overflow-hidden text-ellipsis whitespace-nowrap text-xs text-secondary">{detail}</span>
      {typeof meter === "number" ? (
        <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-background-muted">
          <div className={meterBarClassName(meter)} />
        </div>
      ) : null}
    </div>
  );
}
