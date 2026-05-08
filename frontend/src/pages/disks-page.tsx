import { HardDrive } from "lucide-react";
import { DetailTile, SummaryCard } from "../components/shared/detail-tile";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { Badge } from "../components/ui/badge";
import { Card } from "../components/ui/card";
import { useSystemSnapshotQuery } from "../lib/api-client";
import { formatBytes, formatPercent, formatRate } from "../lib/format";
import { meterBarClassName } from "../lib/meters";

export function DisksPage() {
  const { data, isLoading } = useSystemSnapshotQuery();
  const drives = data?.disk.drives ?? [];

  if (isLoading) {
    return <PageSkeleton />;
  }

  if (drives.length === 0) {
    return <EmptyState icon={HardDrive} title="No drives detected" description="Disk telemetry will populate this page once the collector responds." />;
  }

  const totalBytes = drives.reduce((sum, drive) => sum + drive.total_bytes, 0);
  const usedBytes = drives.reduce((sum, drive) => sum + drive.used_bytes, 0);
  const hottestDrive = [...drives].sort((left, right) => right.used_pct - left.used_pct)[0];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Disks"
        description="Capacity, throughput, and pressure for every visible drive, with the fullest volumes called out first."
        eyebrow="Storage"
        icon={HardDrive}
        meta={
          <>
            <Badge variant="info">{drives.length} drives</Badge>
            <Badge variant={hottestDrive && hottestDrive.used_pct >= 90 ? "warning" : "success"}>
              {hottestDrive ? `${hottestDrive.letter} busiest` : "Healthy"}
            </Badge>
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <SummaryCard label="Total capacity" value={formatBytes(totalBytes)} accent={<Badge variant="neutral">Visible</Badge>} />
        <SummaryCard label="Used capacity" value={formatBytes(usedBytes)} accent={<Badge variant="info">{formatPercent((usedBytes / totalBytes) * 100 || 0)}</Badge>} />
        <SummaryCard
          label="Highest pressure"
          value={hottestDrive ? `${hottestDrive.letter} ${formatPercent(hottestDrive.used_pct)}` : "--"}
          valueClassName="text-2xl font-semibold"
          accent={<Badge variant={hottestDrive && hottestDrive.used_pct >= 90 ? "warning" : "success"}>Pressure</Badge>}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        {drives.map((drive) => (
          <Card key={drive.letter} className="space-y-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="eyebrow">Drive {drive.letter}</div>
                <h2 className="mt-2 text-xl font-semibold tracking-tight text-foreground">{drive.label || drive.letter}</h2>
                <p className="mt-1 text-sm leading-6 text-secondary">{drive.fs_type || "Unknown filesystem"} mounted and tracked by the collector.</p>
              </div>
              <Badge variant={drive.used_pct >= 90 ? "warning" : drive.used_pct >= 75 ? "info" : "success"}>{formatPercent(drive.used_pct)}</Badge>
            </div>

            <div className="meter">
              <div className={meterBarClassName(drive.used_pct)} />
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <DetailTile label="Used" value={`${formatBytes(drive.used_bytes)} / ${formatBytes(drive.total_bytes)}`} hint="Occupied capacity right now" />
              <DetailTile label="Filesystem" value={drive.fs_type || "Unknown"} hint="Reported format from the collector" />
              <DetailTile label="Read throughput" value={formatRate(drive.read_bps)} hint="Current inbound disk rate" />
              <DetailTile label="Write throughput" value={formatRate(drive.write_bps)} hint="Current outbound disk rate" />
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
}
