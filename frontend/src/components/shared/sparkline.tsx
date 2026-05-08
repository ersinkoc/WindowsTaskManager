interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  color?: string;
  fill?: boolean;
  points?: number;
}

export function Sparkline({ data, width = 120, height = 32, color, fill = false, points = 60 }: SparklineProps) {
  if (data.length < 2) return <div style={{ width, height }} className="flex items-center justify-center text-xs text-secondary">--</div>;

  const slice = data.slice(-points);
  const min = Math.min(...slice);
  const max = Math.max(...slice);
  const range = max - min || 1;

  const pad = 2;
  const w = width - pad * 2;
  const h = height - pad * 2;

  const coords = slice.map((v, i) => {
    const x = pad + (i / (slice.length - 1)) * w;
    const y = pad + h - ((v - min) / range) * h;
    return `${x},${y}`;
  });

  const polyline = coords.join(" ");
  const areaPath = `M${coords[0]} L${coords.slice(1).join(" L")} L${width - pad},${height - pad} L${pad},${height - pad} Z`;

  const lineColor = color ?? (max >= 90 ? "var(--error)" : max >= 75 ? "var(--warning)" : "var(--accent)");

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      aria-hidden="true"
      style={{ overflow: "visible" }}
    >
      {fill && (
        <path
          d={areaPath}
          fill={lineColor}
          fillOpacity="0.12"
          stroke="none"
        />
      )}
      <polyline
        points={polyline}
        fill="none"
        stroke={lineColor}
        strokeWidth="1.5"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

interface MiniMetricProps {
  label: string;
  value: string;
  trend?: number[];
  detail?: string;
}

export function MiniMetric({ label, value, trend, detail }: MiniMetricProps) {
  return (
    <div className="flex items-center gap-3">
      <div className="min-w-0">
        <div className="text-[0.65rem] font-semibold uppercase tracking-wider text-secondary">{label}</div>
        <div className="text-sm font-semibold tabular-nums text-foreground">{value}</div>
        {detail && <div className="text-xs text-secondary">{detail}</div>}
      </div>
      {trend && trend.length > 1 && <Sparkline data={trend} width={80} height={28} />}
    </div>
  );
}