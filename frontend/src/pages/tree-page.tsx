import { useMemo, useState } from "react";
import { GitBranch, Search, TriangleAlert } from "lucide-react";
import { useNavigate } from "react-router";
import { ConfirmDialog } from "../components/shared/confirm-dialog";
import { SummaryCard } from "../components/shared/detail-tile";
import { EmptyState } from "../components/shared/empty-state";
import { FilterChip } from "../components/shared/filter-chip";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { SearchInput } from "../components/shared/search-input";
import { Badge } from "../components/ui/badge";
import { Card } from "../components/ui/card";
import { useSystemSnapshotQuery } from "../lib/api-client";
import { useProcessActionMutation } from "../lib/api-client";
import type { ProcessNode } from "../types/api";

export function TreePage() {
  const { data, isLoading } = useSystemSnapshotQuery();
  const navigate = useNavigate();
  const [searchValue, setSearchValue] = useState("");
  const [showOnlyOrphans, setShowOnlyOrphans] = useState(false);
  const [actionTarget, setActionTarget] = useState<{ pid: number; name: string; action: "kill" | "suspend" | "resume" } | null>(null);
  const actionMutation = useProcessActionMutation({ successMessage: false });
  const nodes = data?.process_tree ?? [];
  const flatNodes = useMemo(() => flattenNodes(nodes), [nodes]);
  const query = searchValue.trim().toLowerCase();

  const filteredNodes = useMemo(() => {
    if (!query && !showOnlyOrphans) {
      return nodes;
    }
    return filterTree(nodes, query, showOnlyOrphans);
  }, [nodes, query, showOnlyOrphans]);

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (nodes.length === 0) {
    return <EmptyState icon={GitBranch} title="No process tree yet" description="WTM has not produced a tree snapshot yet. This usually fills in after the next collector cycle." />;
  }

  const totalNodes = flatNodes.length;
  const orphanCount = flatNodes.filter((node) => node.is_orphan).length;
  const deepestLevel = flatNodes.reduce((max, node) => Math.max(max, node.depth), 0);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Process Tree"
        description="Trace parent-child lineage, isolate orphaned branches, and inspect one family without losing wider context."
        eyebrow="Hierarchy"
        icon={GitBranch}
        meta={
          <>
            <Badge variant="info">{totalNodes} processes</Badge>
            <Badge variant={orphanCount > 0 ? "warning" : "success"}>{orphanCount} orphans</Badge>
          </>
        }
        actions={
          <>
            <SearchInput
              ariaLabel="Search the process tree by name or PID"
              placeholder="Search name or PID"
              value={searchValue}
              widthClassName="sm:w-80"
              onChange={setSearchValue}
            />
            <FilterChip active={!showOnlyOrphans} label="Full tree" onClick={() => setShowOnlyOrphans(false)} />
            <FilterChip active={showOnlyOrphans} label="Orphans only" onClick={() => setShowOnlyOrphans(true)} />
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <SummaryCard label="Root nodes" value={String(filteredNodes.length)} accent={<Badge variant="info">Visible roots</Badge>} />
        <SummaryCard label="Processes in tree" value={String(countNodes(filteredNodes))} accent={<Badge variant="neutral">Filtered</Badge>} />
        <SummaryCard label="Depth" value={`${deepestLevel + 1} levels`} accent={<Badge variant="warning">Hierarchy</Badge>} />
      </div>

      {filteredNodes.length === 0 ? (
        <EmptyState icon={Search} title="No branches match" description="Try a different process name, PID, or switch back from orphan-only mode." />
      ) : null}

      <Card className={filteredNodes.length === 0 ? "hidden space-y-0 overflow-hidden p-0" : "space-y-0 overflow-hidden p-0"}>
        <div className="flex flex-col gap-3 border-b border-border px-4 py-4 xl:flex-row xl:items-end xl:justify-between sm:px-5">
          <div>
            <div className="eyebrow">Tree canvas</div>
            <h2 className="mt-2 text-lg font-semibold tracking-tight text-foreground">Process lineage</h2>
            <p className="mt-1 text-sm leading-6 text-secondary">Filtering keeps context around matches so you can see both ancestry and descendants without losing the branch.</p>
          </div>
          <div className="grid gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-3 xl:min-w-[30rem]">
            <GuideRow
              title="Roots"
              description="Branches start where no visible parent is present."
            />
            <GuideRow
              title="Orphans"
              description="Worth checking when parent lineage is missing."
              tone="warning"
            />
            <GuideRow
              title="Search"
              description="Matches keep surrounding context instead of isolating leaves."
            />
          </div>
        </div>
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <Badge variant="info">{countNodes(filteredNodes)} visible nodes</Badge>
          <div className="text-sm text-secondary">Desktop keeps the full tree, mobile collapses to cards.</div>
        </div>
        <div className="hidden px-4 py-4 md:block sm:px-5">
          <ul className="space-y-2 font-mono text-sm text-secondary">
            {filteredNodes.map((node) => (
              <TreeNode key={`${node.process.pid}-${node.process.name}`} depth={0} node={node} navigate={navigate} query={query} onAction={setActionTarget} />
            ))}
          </ul>
        </div>
        <div className="grid gap-3 px-4 py-4 md:hidden sm:px-5">
          {filteredNodes.map((node) => (
            <TreeCard key={`${node.process.pid}-${node.process.name}`} node={node} navigate={navigate} query={query} />
          ))}
        </div>
      </Card>
      <ConfirmDialog
        open={actionTarget !== null}
        title={actionTarget ? `${actionTarget.action.charAt(0).toUpperCase() + actionTarget.action.slice(1)} ${actionTarget.name}?` : "Confirm action"}
        description={
          actionTarget
            ? `This will ${actionTarget.action} PID ${actionTarget.pid} (${actionTarget.name}).`
            : "Confirm this process action."
        }
        confirmLabel={actionTarget ? actionTarget.action.charAt(0).toUpperCase() + actionTarget.action.slice(1) : "Confirm"}
        tone={actionTarget?.action === "kill" ? "danger" : "neutral"}
        isPending={actionMutation.isPending}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
        onConfirm={() => {
          if (!actionTarget) return;
          actionMutation.mutate(
            { pid: actionTarget.pid, action: actionTarget.action },
            { onSuccess: () => setActionTarget(null) },
          );
        }}
      />
    </div>
  );
}

interface TreeNodeProps {
  depth: number;
  node: ProcessNode;
  navigate: ReturnType<typeof useNavigate>;
  query?: string;
  onAction?: (target: { pid: number; name: string; action: "kill" | "suspend" | "resume" }) => void;
}

function TreeNode({ depth, node, navigate, query, onAction }: TreeNodeProps) {
  return (
    <li className={depth > 0 ? "border-l border-border pl-4" : ""}>
      <div className="rounded-md border border-border bg-surface px-4 py-2.5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold text-foreground">{query ? <HighlightedText text={node.process.name} query={query} /> : node.process.name}</span>
          <button
            type="button"
            className="text-xs text-secondary hover:text-accent transition-colors"
            onClick={() => navigate(`/processes?pid=${node.process.pid}`)}
            title={`View PID ${node.process.pid} in processes`}
          >
            PID {node.process.pid}
          </button>
          {node.is_orphan ? <Badge variant="warning">orphan</Badge> : null}
          {node.children?.length ? <Badge variant="neutral">{node.children.length} child</Badge> : null}
          {onAction && (
            <div className="ml-auto flex gap-1">
              <button
                type="button"
                className="text-xs text-secondary hover:text-accent transition-colors"
                onClick={() => onAction({ pid: node.process.pid, name: node.process.name, action: "suspend" })}
                title="Suspend"
              >
                ⏸
              </button>
              <button
                type="button"
                className="text-xs text-secondary hover:text-accent transition-colors"
                onClick={() => onAction({ pid: node.process.pid, name: node.process.name, action: "resume" })}
                title="Resume"
              >
                ▶
              </button>
              <button
                type="button"
                className="text-xs text-error hover:text-error transition-colors"
                onClick={() => onAction({ pid: node.process.pid, name: node.process.name, action: "kill" })}
                title="Kill"
              >
                ✕
              </button>
            </div>
          )}
        </div>
      </div>
      {node.children?.length ? (
        <ul className="mt-2 space-y-2">
          {node.children.map((child) => (
            <TreeNode key={`${child.process.pid}-${child.process.name}`} depth={depth + 1} node={child} navigate={navigate} query={query} onAction={onAction} />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

interface TreeCardProps {
  node: ProcessNode;
  navigate: ReturnType<typeof useNavigate>;
  query?: string;
}

function TreeCard({ node, navigate, query }: TreeCardProps) {
  return (
    <Card className="space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-lg font-semibold tracking-tight text-foreground">{query ? <HighlightedText text={node.process.name} query={query} /> : node.process.name}</div>
          <button
            type="button"
            className="mt-1 font-mono text-sm text-secondary hover:text-accent transition-colors"
            onClick={() => navigate(`/processes?pid=${node.process.pid}`)}
            title={`View PID ${node.process.pid} in processes`}
          >
            PID {node.process.pid}
          </button>
        </div>
        <Badge variant={node.is_orphan ? "warning" : "info"}>{node.is_orphan ? "Orphan" : `${node.children?.length ?? 0} child`}</Badge>
      </div>
      {node.children && node.children.length > 0 ? (
        <div className="space-y-2">
          <div className="metric-label">Children</div>
          <div className="grid gap-2">
            {node.children.map((child) => (
              <div key={`${child.process.pid}-${child.process.name}`} className="rounded-md border border-border bg-surface px-3 py-3">
                <div className="text-sm font-semibold text-foreground">{child.process.name}</div>
                <div className="mt-1 font-mono text-xs text-secondary">PID {child.process.pid}</div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="rounded-md border border-border bg-surface px-3 py-3 text-sm text-secondary">No child processes under this branch root.</div>
      )}
    </Card>
  );
}

interface GuideRowProps {
  title: string;
  description: string;
  tone?: "neutral" | "warning";
}

function GuideRow({ title, description, tone = "neutral" }: GuideRowProps) {
  return (
    <div className="bg-surface px-4 py-3">
      <div className="flex items-center gap-2">
        {tone === "warning" ? <TriangleAlert className="h-4 w-4 text-warning" /> : <GitBranch className="h-4 w-4 text-accent" />}
        <div className="text-sm font-semibold text-foreground">{title}</div>
      </div>
      <div className="mt-1.5 text-sm leading-6 text-secondary">{description}</div>
    </div>
  );
}

function countNodes(nodes: ProcessNode[]) {
  let total = 0;
  for (const node of nodes) {
    total += 1;
    if (node.children && node.children.length > 0) {
      total += countNodes(node.children);
    }
  }
  return total;
}

function flattenNodes(nodes: ProcessNode[], depth = 0): Array<ProcessNode & { depth: number }> {
  return nodes.flatMap((node) => [
    { ...node, depth },
    ...(node.children ? flattenNodes(node.children, depth + 1) : []),
  ]);
}

function filterTree(nodes: ProcessNode[], query: string, showOnlyOrphans: boolean): ProcessNode[] {
  const output: ProcessNode[] = [];
  for (const node of nodes) {
    const children = filterTree(node.children ?? [], query, showOnlyOrphans);
    const matchesQuery = !query || node.process.name.toLowerCase().includes(query) || String(node.process.pid).includes(query);
    const matchesOrphan = !showOnlyOrphans || node.is_orphan;
    if ((matchesQuery && matchesOrphan) || children.length > 0) {
      output.push({
        ...node,
        children: children.length > 0 ? children : undefined,
      });
    }
  }
  return output;
}

function HighlightedText({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;
  const idx = text.toLowerCase().indexOf(query.toLowerCase());
  if (idx < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="bg-warning/30 text-foreground rounded-sm px-0.5">{text.slice(idx, idx + query.length)}</mark>
      {text.slice(idx + query.length)}
    </>
  );
}
