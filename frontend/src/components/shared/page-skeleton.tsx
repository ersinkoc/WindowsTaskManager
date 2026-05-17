export function PageSkeleton() {
  return (
    <div className="space-y-6 animate-in fade-in duration-300">
      <div className="h-8 w-48 rounded-xl bg-background-muted animate-pulse" />
      <div className="stat-grid">
        {Array.from({ length: 4 }, (_, index) => (
          <div
            key={index}
            className="h-32 rounded-2xl border border-border bg-background-subtle animate-pulse"
            style={{
              animationDelay: `${index * 100}ms`,
              animationDuration: "1.5s",
            }}
          />
        ))}
      </div>
      <div
        className="h-96 rounded-2xl border border-border bg-background-subtle animate-pulse"
        style={{
          animationDelay: "200ms",
          animationDuration: "2s",
        }}
      />
    </div>
  );
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-3 animate-in fade-in duration-300">
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className="flex items-center gap-4 rounded-xl border border-border bg-background-subtle p-4 animate-pulse"
          style={{
            animationDelay: `${index * 50}ms`,
            animationDuration: "1.2s",
          }}
        >
          <div className="h-4 w-4 rounded bg-background-muted" />
          <div className="h-4 flex-1 rounded bg-background-muted" />
          <div className="h-4 w-20 rounded bg-background-muted" />
          <div className="h-4 w-16 rounded bg-background-muted" />
        </div>
      ))}
    </div>
  );
}

export function ChartSkeleton() {
  return (
    <div className="space-y-4 animate-in fade-in duration-300">
      <div className="flex items-end justify-between gap-2 rounded-2xl border border-border bg-background-subtle p-6 h-64 animate-pulse">
        {Array.from({ length: 12 }, (_, index) => (
          <div
            key={index}
            className="w-full rounded-t bg-background-muted"
            style={{
              height: `${30 + Math.random() * 60}%`,
              animationDelay: `${index * 50}ms`,
              animationDuration: "1.5s",
            }}
          />
        ))}
      </div>
      <div className="flex justify-between px-2">
        {Array.from({ length: 6 }, (_, index) => (
          <div key={index} className="h-3 w-16 rounded bg-background-muted animate-pulse" style={{ animationDelay: `${index * 80}ms` }} />
        ))}
      </div>
    </div>
  );
}

export function DetailSkeleton() {
  return (
    <div className="grid gap-4 animate-in fade-in duration-300 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: 6 }, (_, index) => (
        <div key={index} className="space-y-2 rounded-2xl border border-border bg-background-subtle p-4 animate-pulse">
          <div className="h-3 w-20 rounded bg-background-muted" />
          <div className="h-6 w-24 rounded bg-background-muted" />
        </div>
      ))}
    </div>
  );
}