import { useMemo, useState } from "react";
import { BellRing, ShieldAlert } from "lucide-react";
import { useNavigate } from "react-router";
import { SummaryCard } from "../components/shared/detail-tile";
import { EmptyState } from "../components/shared/empty-state";
import { FilterChip } from "../components/shared/filter-chip";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { ConfirmDialog } from "../components/shared/confirm-dialog";
import { SearchInput } from "../components/shared/search-input";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { useDebouncedValue } from "../hooks/use-debounced-value";
import { useAlertActionMutation, useAlertsQuery } from "../lib/api-client";
import type { AlertItem } from "../types/api";

type AlertFilter = "all" | "critical" | "warning" | "info";

const PAGE_SIZE = 20;

export function AlertsPage() {
  const { data, isLoading } = useAlertsQuery();
  const alertActionMutation = useAlertActionMutation();
  const navigate = useNavigate();
  const [dismissCandidate, setDismissCandidate] = useState<AlertItem | null>(null);
  const [searchValue, setSearchValue] = useState("");
  const [filter, setFilter] = useState<AlertFilter>("all");
  const [historyPage, setHistoryPage] = useState(1);
  const debouncedSearch = useDebouncedValue(searchValue, 300);
  const activeAlerts = data?.active ?? [];
  const historyAlerts = data?.history ?? [];

  const filteredActive = useMemo(() => filterAlerts(activeAlerts, debouncedSearch, filter), [activeAlerts, debouncedSearch, filter]);
  const filteredHistoryAll = useMemo(() => filterAlerts(historyAlerts, debouncedSearch, filter), [historyAlerts, debouncedSearch, filter]);
  const paginatedHistory = useMemo(() => filteredHistoryAll.slice(0, historyPage * PAGE_SIZE), [filteredHistoryAll, historyPage]);
  const hasMoreHistory = paginatedHistory.length < filteredHistoryAll.length;

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (!data || (activeAlerts.length === 0 && historyAlerts.length === 0)) {
    return <EmptyState icon={ShieldAlert} title="No alerts yet" description="When anomaly detectors fire, active and historical alerts will appear here." />;
  }

  const criticalCount = activeAlerts.filter((alert) => alert.severity === "critical").length;
  const warningCount = activeAlerts.filter((alert) => alert.severity === "warning").length;
  const infoCount = activeAlerts.filter((alert) => alert.severity === "info").length;

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="Alerts"
          description="Current and historical anomaly alerts from the Go engine, separated so urgent signals stay readable."
          eyebrow="Anomaly stream"
          icon={ShieldAlert}
          meta={
            <>
              <Badge variant={criticalCount > 0 ? "error" : "success"}>{criticalCount} critical</Badge>
              <Badge variant={warningCount > 0 ? "warning" : "neutral"}>{warningCount} warning</Badge>
              <Badge variant="info">{historyAlerts.length} history</Badge>
            </>
          }
          actions={
            <SearchInput
              ariaLabel="Search alerts by title, type, severity, or PID"
              placeholder="Search alerts"
              value={searchValue}
              widthClassName="sm:w-80"
              onChange={setSearchValue}
            />
          }
        />

        <div className="grid gap-4 sm:grid-cols-3">
          <SummaryCard
            label="Active alerts"
            value={String(activeAlerts.length)}
            accent={<Badge variant={activeAlerts.length > 0 ? "warning" : "neutral"}>Active alerts</Badge>}
          />
          <SummaryCard
            label="Critical active"
            value={String(criticalCount)}
            accent={<Badge variant={criticalCount > 0 ? "error" : "neutral"}>Critical active</Badge>}
          />
          <SummaryCard
            label="History events"
            value={String(historyAlerts.length)}
            accent={<Badge variant={historyAlerts.length > 0 ? "info" : "neutral"}>History events</Badge>}
          />
        </div>

        {filteredActive.length === 0 && paginatedHistory.length === 0 ? (
          <EmptyState icon={ShieldAlert} title="No alerts match" description="Try a different title, PID, severity, or type filter." />
        ) : null}

        <Card className="space-y-0 overflow-hidden p-0">
          <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:px-5 xl:flex-row xl:items-center xl:justify-between">
            <div>
              <div className="eyebrow">Alert queue</div>
              <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground">Active and recent alerts</h2>
              <p className="mt-1 text-sm leading-6 text-secondary">Triage active signals first, then use recent history as context instead of letting every alert compete for attention.</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <FilterChip active={filter === "all"} label="All" onClick={() => setFilter("all")} />
              <FilterChip active={filter === "critical"} label="Critical" onClick={() => setFilter("critical")} />
              <FilterChip active={filter === "warning"} label="Warning" onClick={() => setFilter("warning")} />
              <FilterChip active={filter === "info"} label="Info" onClick={() => setFilter("info")} />
            </div>
          </div>

          <div className="grid gap-px border-b border-border bg-border sm:grid-cols-3">
            <SeverityRow label="Critical" value={criticalCount} variant="error" />
            <SeverityRow label="Warning" value={warningCount} variant="warning" />
            <SeverityRow label="Info" value={infoCount} variant="info" />
          </div>

          <div className={filteredActive.length === 0 ? "hidden" : "space-y-2"}>
            <div className="flex items-center justify-between gap-3 px-4 pt-4 sm:px-5">
              <h2 className="section-title">Active</h2>
              <Badge variant={warningCount > 0 ? "warning" : "info"}>{filteredActive.length} shown</Badge>
            </div>
            <div className="overflow-hidden border-y border-border bg-background">
              {filteredActive.map((alert, index) => (
                <AlertRow
                  key={`${alert.type}-${alert.pid ?? "global"}-${alert.title}`}
                  alert={alert}
                  isPending={alertActionMutation.isPending}
                  isLast={index === filteredActive.length - 1}
                  onDismiss={() => setDismissCandidate(alert)}
                  onSnooze={() => alertActionMutation.mutate({ type: alert.type, pid: alert.pid, action: "snooze" })}
                  navigate={navigate}
                />
              ))}
            </div>
          </div>

          <div className={paginatedHistory.length === 0 ? "hidden" : "space-y-2"}>
            <div className="flex items-center justify-between gap-3 px-4 pt-4 sm:px-5">
              <div className="flex items-center gap-3">
                <h2 className="section-title">History</h2>
                {filteredHistoryAll.length > 0 && (
                  <Badge variant="neutral">Total {filteredHistoryAll.length}</Badge>
                )}
              </div>
              <Badge variant="info">{paginatedHistory.length} shown</Badge>
            </div>
            <div className="overflow-hidden border-y border-border bg-background">
              {paginatedHistory.map((alert, index) => (
                <AlertRow key={`${alert.type}-${alert.pid ?? "global"}-${alert.title}`} alert={alert} isLast={index === paginatedHistory.length - 1} navigate={navigate} />
              ))}
            </div>
            {hasMoreHistory ? (
              <div className="flex justify-center px-4 pb-4 sm:px-5">
                <Button type="button" variant="secondary" onClick={() => setHistoryPage((p) => p + 1)}>
                  Load more ({filteredHistoryAll.length - paginatedHistory.length} remaining)
                </Button>
              </div>
            ) : null}
          </div>
        </Card>
      </div>
      <ConfirmDialog
        open={dismissCandidate !== null}
        title={dismissCandidate ? `Dismiss ${dismissCandidate.title}?` : "Dismiss alert?"}
        description={
          dismissCandidate
            ? `This hides the active ${dismissCandidate.severity} alert "${dismissCandidate.title}"${dismissCandidate.pid ? ` for PID ${dismissCandidate.pid}` : ""}.`
            : "This hides the selected active alert."
        }
        confirmLabel="Dismiss alert"
        tone="neutral"
        isPending={alertActionMutation.isPending}
        onOpenChange={(open) => {
          if (!open) {
            setDismissCandidate(null);
          }
        }}
        onConfirm={() => {
          if (!dismissCandidate) {
            return;
          }
          alertActionMutation.mutate(
            { type: dismissCandidate.type, pid: dismissCandidate.pid, action: "dismiss" },
            {
              onSuccess: () => {
                setDismissCandidate(null);
              },
            },
          );
        }}
      />
    </>
  );
}

interface AlertRowProps {
  alert: AlertItem;
  isPending?: boolean;
  isLast?: boolean;
  onDismiss?: () => void;
  onSnooze?: () => void;
  navigate?: ReturnType<typeof useNavigate>;
}

function AlertRow({ alert, isPending = false, isLast = false, onDismiss, onSnooze, navigate }: AlertRowProps) {
  const variant = alert.severity === "critical" ? "error" : alert.severity === "warning" ? "warning" : "info";

  return (
    <div className={`grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1fr)_auto] ${isLast ? "" : "border-b border-border"}`}>
      <div className="min-w-0 space-y-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge variant={variant}>{alert.severity}</Badge>
          <Badge variant="neutral">{alert.type}</Badge>
          {alert.pid && navigate ? (
            <button
              type="button"
              className="hover:text-accent text-neutral transition-colors text-xs font-mono"
              onClick={() => navigate(`/processes?pid=${alert.pid}`)}
              title={`View PID ${alert.pid} in processes`}
            >
              PID {alert.pid}
            </button>
          ) : alert.pid ? (
            <Badge variant="neutral">PID {alert.pid}</Badge>
          ) : null}
          <div className="min-w-0 truncate text-sm font-semibold text-foreground">{alert.title}</div>
        </div>
        <p className="text-sm leading-5 text-secondary">{alert.description}</p>
      </div>
      {onDismiss || onSnooze ? (
        <div className="flex flex-wrap items-start gap-2 lg:justify-end">
          {onSnooze ? (
            <Button type="button" size="sm" variant="secondary" disabled={isPending} onClick={onSnooze}>
              Snooze 30m
            </Button>
          ) : null}
          {onDismiss ? (
            <Button type="button" size="sm" variant="ghost" disabled={isPending} onClick={onDismiss}>
              Dismiss
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

interface SeverityRowProps {
  label: string;
  value: number;
  variant: "info" | "warning" | "error";
}

function SeverityRow({ label, value, variant }: SeverityRowProps) {
  return (
    <div className="flex items-center justify-between gap-3 bg-surface px-4 py-3 sm:px-5">
      <div className="flex min-w-0 items-center gap-2">
        <BellRing className="h-4 w-4 text-accent" />
        <span className="truncate text-sm font-semibold text-foreground">{label}</span>
      </div>
      <Badge variant={variant}>{value}</Badge>
    </div>
  );
}

function filterAlerts(alerts: AlertItem[], query: string, filter: AlertFilter) {
  const needle = query.trim().toLowerCase();
  return alerts.filter((alert) => {
    if (filter !== "all" && alert.severity !== filter) {
      return false;
    }
    if (!needle) {
      return true;
    }
    return (
      alert.title.toLowerCase().includes(needle) ||
      alert.description.toLowerCase().includes(needle) ||
      alert.type.toLowerCase().includes(needle) ||
      alert.severity.toLowerCase().includes(needle) ||
      String(alert.pid ?? "").includes(needle)
    );
  });
}
