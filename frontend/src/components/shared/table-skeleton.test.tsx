import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChartSkeleton, DetailSkeleton, PageSkeleton, TableSkeleton } from "./page-skeleton";

describe("TableSkeleton", () => {
  it("renders the default 5 rows when no rows prop is provided", () => {
    const { container } = render(<TableSkeleton />);
    // Each row is a flex container with 4 inner children (icon + 3 fields).
    // The top-level wrapper contains 5 such rows.
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("space-y-3", "animate-in", "fade-in");
    const rows = root.children;
    expect(rows).toHaveLength(5);
  });

  it("renders exactly the number of rows requested via the rows prop", () => {
    const { container } = render(<TableSkeleton rows={3} />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.children).toHaveLength(3);
  });

  it("renders zero rows when rows=0", () => {
    const { container } = render(<TableSkeleton rows={0} />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.children).toHaveLength(0);
  });

  it("renders many rows when rows is large", () => {
    const { container } = render(<TableSkeleton rows={20} />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.children).toHaveLength(20);
  });

  it("each row has 4 placeholder cells", () => {
    const { container } = render(<TableSkeleton rows={2} />);
    const rows = (container.firstElementChild as HTMLElement).children;
    for (const row of Array.from(rows)) {
      expect(row.children).toHaveLength(4);
    }
  });

  it("applies the staggered animationDelay/animationDuration styles to each row", () => {
    const { container } = render(<TableSkeleton rows={4} />);
    const rows = (container.firstElementChild as HTMLElement).children;
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i] as HTMLElement;
      expect(row.style.animationDelay).toBe(`${i * 50}ms`);
      expect(row.style.animationDuration).toBe("1.2s");
    }
  });

  it("uses stable React keys by index (no duplicate keys emitted)", () => {
    const { container } = render(<TableSkeleton rows={3} />);
    // Keys are React-internal; their visible effect is a clean DOM with
    // exactly `rows` row nodes (no orphan duplicates).
    const root = container.firstElementChild as HTMLElement;
    expect(root.children).toHaveLength(3);
  });

  it("applies the row-level utility classes", () => {
    const { container } = render(<TableSkeleton rows={1} />);
    const row = (container.firstElementChild as HTMLElement).firstElementChild as HTMLElement;
    expect(row).toHaveClass("flex");
    expect(row).toHaveClass("items-center");
    expect(row).toHaveClass("gap-4");
    expect(row).toHaveClass("rounded-xl");
    expect(row).toHaveClass("border");
    expect(row).toHaveClass("bg-background-subtle");
    expect(row).toHaveClass("p-4");
    expect(row).toHaveClass("animate-pulse");
  });

  it("renders a placeholder icon and three skeleton fields in each row", () => {
    const { container } = render(<TableSkeleton rows={1} />);
    const row = (container.firstElementChild as HTMLElement).firstElementChild as HTMLElement;
    const cells = Array.from(row.children);
    expect(cells[0]).toHaveClass("h-4", "w-4");
    expect(cells[1]).toHaveClass("h-4", "flex-1");
    expect(cells[2]).toHaveClass("h-4", "w-20");
    expect(cells[3]).toHaveClass("h-4", "w-16");
  });

  it("renders without throwing when wrapped in a fragment-like host", () => {
    expect(() =>
      render(
        <div>
          <TableSkeleton rows={1} />
        </div>,
      ),
    ).not.toThrow();
  });

  it("places the skeleton output inside the host element when nested", () => {
    render(
      <div data-testid="host">
        <TableSkeleton rows={2} />
      </div>,
    );
    const host = screen.getByTestId("host");
    // The host's direct child is the TableSkeleton wrapper div.
    expect(host.firstElementChild).toHaveClass("space-y-3");
    expect(host.firstElementChild?.children).toHaveLength(2);
  });

  it("respects negative-ish but valid row counts (1)", () => {
    const { container } = render(<TableSkeleton rows={1} />);
    expect((container.firstElementChild as HTMLElement).children).toHaveLength(1);
  });

  it("respects large row counts (50) without dropping rows", () => {
    const { container } = render(<TableSkeleton rows={50} />);
    expect((container.firstElementChild as HTMLElement).children).toHaveLength(50);
  });
});

describe("PageSkeleton", () => {
  it("renders a top-level wrapper with the fade-in animation classes", () => {
    const { container } = render(<PageSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("space-y-6", "animate-in", "fade-in", "duration-300");
  });

  it("renders a header placeholder with the muted pulse background", () => {
    const { container } = render(<PageSkeleton />);
    const header = (container.firstElementChild as HTMLElement).firstElementChild as HTMLElement;
    expect(header).toHaveClass("h-8", "w-48", "rounded-xl", "bg-background-muted", "animate-pulse");
  });

  it("renders exactly 4 stat tiles inside the stat grid", () => {
    const { container } = render(<PageSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const statGrid = root.children[1] as HTMLElement;
    expect(statGrid).toHaveClass("stat-grid");
    expect(statGrid.children).toHaveLength(4);
  });

  it("stagger-animates the stat tiles (index * 100ms, duration 1.5s)", () => {
    const { container } = render(<PageSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const statGrid = root.children[1] as HTMLElement;
    for (let i = 0; i < statGrid.children.length; i++) {
      const tile = statGrid.children[i] as HTMLElement;
      expect(tile.style.animationDelay).toBe(`${i * 100}ms`);
      expect(tile.style.animationDuration).toBe("1.5s");
      expect(tile).toHaveClass("h-32", "rounded-2xl", "border", "border-border", "bg-background-subtle", "animate-pulse");
    }
  });

  it("renders a large chart-shaped placeholder after the stat grid", () => {
    const { container } = render(<PageSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const chart = root.children[2] as HTMLElement;
    expect(chart).toHaveClass("h-96", "rounded-2xl", "border", "border-border", "bg-background-subtle", "animate-pulse");
    expect(chart.style.animationDelay).toBe("200ms");
    expect(chart.style.animationDuration).toBe("2s");
  });
});

describe("ChartSkeleton", () => {
  afterEach(() => {
    // Restore Math.random for any other tests that may run after this file.
    vi.restoreAllMocks();
  });

  it("renders a top-level wrapper with fade-in animation classes", () => {
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("space-y-4", "animate-in", "fade-in", "duration-300");
  });

  it("renders the bar container with 12 bars", () => {
    // Pin Math.random so the bar heights are deterministic.
    vi.spyOn(Math, "random").mockReturnValue(0.5);
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const barContainer = root.firstElementChild as HTMLElement;
    expect(barContainer).toHaveClass("flex", "items-end", "justify-between", "gap-2");
    expect(barContainer).toHaveClass("rounded-2xl", "border", "border-border", "bg-background-subtle");
    expect(barContainer).toHaveClass("p-6", "h-64", "animate-pulse");
    expect(barContainer.children).toHaveLength(12);
  });

  it("computes each bar height from Math.random (deterministic when mocked)", () => {
    vi.spyOn(Math, "random").mockReturnValue(0.5);
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const barContainer = root.firstElementChild as HTMLElement;
    for (const bar of Array.from(barContainer.children)) {
      // height = `${30 + Math.random() * 60}%` → with random=0.5 → "60%"
      expect((bar as HTMLElement).style.height).toBe("60%");
    }
  });

  it("stagger-animates the bars (index * 50ms, duration 1.5s)", () => {
    vi.spyOn(Math, "random").mockReturnValue(0.25);
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const barContainer = root.firstElementChild as HTMLElement;
    for (let i = 0; i < barContainer.children.length; i++) {
      const bar = barContainer.children[i] as HTMLElement;
      expect(bar.style.animationDelay).toBe(`${i * 50}ms`);
      expect(bar.style.animationDuration).toBe("1.5s");
      expect(bar).toHaveClass("w-full", "rounded-t", "bg-background-muted");
    }
  });

  it("renders 6 x-axis tick placeholders below the chart", () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const axisRow = root.children[1] as HTMLElement;
    expect(axisRow).toHaveClass("flex", "justify-between", "px-2");
    expect(axisRow.children).toHaveLength(6);
  });

  it("stagger-animates the x-axis ticks (index * 80ms)", () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const { container } = render(<ChartSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    const axisRow = root.children[1] as HTMLElement;
    for (let i = 0; i < axisRow.children.length; i++) {
      const tick = axisRow.children[i] as HTMLElement;
      expect(tick.style.animationDelay).toBe(`${i * 80}ms`);
      expect(tick).toHaveClass("h-3", "w-16", "rounded", "bg-background-muted", "animate-pulse");
    }
  });
});

describe("DetailSkeleton", () => {
  it("renders a top-level responsive grid wrapper", () => {
    const { container } = render(<DetailSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("grid", "gap-4", "animate-in", "fade-in", "duration-300");
    expect(root).toHaveClass("sm:grid-cols-2", "lg:grid-cols-3");
  });

  it("renders exactly 6 detail cards", () => {
    const { container } = render(<DetailSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.children).toHaveLength(6);
  });

  it("each card has a label placeholder and a value placeholder", () => {
    const { container } = render(<DetailSkeleton />);
    const root = container.firstElementChild as HTMLElement;
    for (const card of Array.from(root.children)) {
      const cardEl = card as HTMLElement;
      expect(cardEl).toHaveClass("space-y-2", "rounded-2xl", "border", "border-border", "bg-background-subtle");
      expect(cardEl).toHaveClass("p-4", "animate-pulse");
      const [label, value] = Array.from(cardEl.children);
      expect(label).toHaveClass("h-3", "w-20", "rounded", "bg-background-muted");
      expect(value).toHaveClass("h-6", "w-24", "rounded", "bg-background-muted");
    }
  });

  it("renders cleanly when nested in another host element", () => {
    render(
      <div data-testid="host">
        <DetailSkeleton />
      </div>,
    );
    const host = screen.getByTestId("host");
    expect(host.firstElementChild).toHaveClass("grid");
    expect(host.firstElementChild?.children).toHaveLength(6);
  });
});