import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MiniMetric, Sparkline } from "./sparkline";

const ascending = [10, 20, 30, 40, 50, 60, 70, 80];
const descending = [80, 70, 60, 50, 40, 30, 20, 10];
const flat = [42, 42, 42, 42, 42, 42];

describe("Sparkline", () => {
  describe("placeholder behavior when data is too short", () => {
    it("renders a '--' placeholder when data is empty", () => {
      const { container } = render(<Sparkline data={[]} />);
      const placeholder = container.querySelector("div");
      expect(placeholder).not.toBeNull();
      expect(placeholder).toHaveTextContent("--");
      // The placeholder must use the default width/height (120x32)
      expect(placeholder).toHaveStyle({ width: "120px", height: "32px" });
      // It must not render an svg
      expect(container.querySelector("svg")).toBeNull();
    });

    it("renders a '--' placeholder when data has exactly one point", () => {
      const { container } = render(<Sparkline data={[50]} />);
      const placeholder = container.querySelector("div");
      expect(placeholder).not.toBeNull();
      expect(placeholder).toHaveTextContent("--");
      expect(container.querySelector("svg")).toBeNull();
    });

    it("uses the provided width/height on the placeholder", () => {
      const { container } = render(<Sparkline data={[]} width={200} height={48} />);
      const placeholder = container.firstElementChild as HTMLElement;
      expect(placeholder).toHaveStyle({ width: "200px", height: "48px" });
    });
  });

  describe("svg rendering when data is sufficient", () => {
    it("renders an svg with default width/height when data has 2+ points", () => {
      const { container } = render(<Sparkline data={ascending} />);
      const svg = container.querySelector("svg");
      expect(svg).not.toBeNull();
      expect(svg).toHaveAttribute("width", "120");
      expect(svg).toHaveAttribute("height", "32");
      expect(svg).toHaveAttribute("viewBox", "0 0 120 32");
      expect(svg).toHaveAttribute("aria-hidden", "true");
    });

    it("respects custom width and height", () => {
      const { container } = render(<Sparkline data={ascending} width={240} height={64} />);
      const svg = container.querySelector("svg");
      expect(svg).toHaveAttribute("width", "240");
      expect(svg).toHaveAttribute("height", "64");
      expect(svg).toHaveAttribute("viewBox", "0 0 240 64");
    });

    it("always renders exactly one polyline", () => {
      const { container } = render(<Sparkline data={ascending} />);
      expect(container.querySelectorAll("polyline")).toHaveLength(1);
    });

    it("does not render a fill path when fill is omitted/false", () => {
      const { container } = render(<Sparkline data={ascending} />);
      expect(container.querySelectorAll("path")).toHaveLength(0);
    });

    it("renders a fill path when fill=true", () => {
      const { container } = render(<Sparkline data={ascending} fill />);
      const paths = container.querySelectorAll("path");
      expect(paths).toHaveLength(1);
      const path = paths[0]!;
      expect(path.getAttribute("d")).toMatch(/^M/);
      expect(path.getAttribute("d")).toMatch(/Z$/);
      expect(path.getAttribute("fill-opacity")).toBe("0.12");
      expect(path.getAttribute("stroke")).toBe("none");
    });

    it("encodes ascending data points across the polyline attribute", () => {
      const { container } = render(<Sparkline data={ascending} width={120} height={32} />);
      const polyline = container.querySelector("polyline");
      const points = polyline?.getAttribute("points") ?? "";
      // 8 data points should produce 8 x,y coordinate pairs in the polyline
      const coords = points.trim().split(/\s+/);
      expect(coords).toHaveLength(8);
      // First coordinate x must sit at the left padding (2px)
      const firstX = Number(coords[0]!.split(",")[0]);
      const lastX = Number(coords[7]!.split(",")[0]);
      expect(firstX).toBe(2);
      // Last x must be width - padding
      expect(lastX).toBe(120 - 2);
      // Ascending data → first y must be lower (greater number) than last y
      const firstY = Number(coords[0]!.split(",")[1]);
      const lastY = Number(coords[7]!.split(",")[1]);
      expect(firstY).toBeGreaterThan(lastY);
    });

    it("encodes descending data points so the first y is above the last y", () => {
      const { container } = render(<Sparkline data={descending} width={120} height={32} />);
      const polyline = container.querySelector("polyline");
      const points = polyline?.getAttribute("points") ?? "";
      const coords = points.trim().split(/\s+/);
      const firstY = Number(coords[0]!.split(",")[1]);
      const lastY = Number(coords[coords.length - 1]!.split(",")[1]);
      expect(firstY).toBeLessThan(lastY);
    });
  });

  describe("slice behavior when data exceeds the points window", () => {
    it("uses only the last N points when data length exceeds the points prop", () => {
      const longData = Array.from({ length: 100 }, (_, i) => i);
      const { container } = render(<Sparkline data={longData} points={5} />);
      const polyline = container.querySelector("polyline");
      const points = polyline?.getAttribute("points") ?? "";
      const coords = points.trim().split(/\s+/);
      // 5 points window → exactly 5 coord pairs
      expect(coords).toHaveLength(5);
      // The last data values 95..99 are the active window.
      const lastX = Number(coords[4]!.split(",")[0]);
      expect(lastX).toBe(118); // width 120 minus padding 2
    });

    it("uses the full dataset when length is below the points prop", () => {
      const { container } = render(<Sparkline data={ascending} points={60} />);
      const polyline = container.querySelector("polyline");
      const points = polyline?.getAttribute("points") ?? "";
      const coords = points.trim().split(/\s+/);
      expect(coords).toHaveLength(ascending.length);
    });
  });

  describe("flat data (range === 0)", () => {
    it("does not throw on flat input and still renders a polyline", () => {
      const { container } = render(<Sparkline data={flat} />);
      const polyline = container.querySelector("polyline");
      expect(polyline).not.toBeNull();
      // With range forced to 1, every y should sit at h - pad (the bottom).
      const points = polyline?.getAttribute("points") ?? "";
      const coords = points.trim().split(/\s+/);
      for (const coord of coords) {
        const y = Number(coord.split(",")[1]);
        // height=32, padding=2 → bottom y is 30
        expect(y).toBe(30);
      }
    });
  });

  describe("color selection", () => {
    it("uses the explicit color when provided", () => {
      const { container } = render(<Sparkline data={ascending} color="#ff00ff" />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("stroke")).toBe("#ff00ff");
    });

    it("uses an explicit color even when the data range would trigger a default tier", () => {
      // max would be 95 → default would be var(--error), but explicit color wins
      const { container } = render(<Sparkline data={[10, 95]} color="#abcdef" />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("stroke")).toBe("#abcdef");
    });

    it("uses var(--error) when max >= 90 and no color is provided", () => {
      const { container } = render(<Sparkline data={[10, 50, 95]} />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("stroke")).toBe("var(--error)");
    });

    it("uses var(--warning) when 75 <= max < 90 and no color is provided", () => {
      const { container } = render(<Sparkline data={[10, 50, 80]} />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("stroke")).toBe("var(--warning)");
    });

    it("uses var(--accent) when max < 75 and no color is provided", () => {
      const { container } = render(<Sparkline data={[10, 20, 30]} />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("stroke")).toBe("var(--accent)");
    });

    it("applies the chosen color to the fill path as well", () => {
      const { container } = render(<Sparkline data={[10, 50, 95]} fill />);
      const path = container.querySelector("path");
      expect(path?.getAttribute("fill")).toBe("var(--error)");
    });
  });

  describe("stroke and viewBox attributes", () => {
    it("sets the expected stroke attributes on the polyline", () => {
      const { container } = render(<Sparkline data={ascending} />);
      const polyline = container.querySelector("polyline");
      expect(polyline?.getAttribute("fill")).toBe("none");
      expect(polyline?.getAttribute("stroke-width")).toBe("1.5");
      expect(polyline?.getAttribute("stroke-linejoin")).toBe("round");
      expect(polyline?.getAttribute("stroke-linecap")).toBe("round");
    });
  });
});

describe("MiniMetric", () => {
  it("renders label and value when trend/detail are omitted", () => {
    render(<MiniMetric label="CPU" value="42%" />);
    expect(screen.getByText("CPU")).toBeInTheDocument();
    expect(screen.getByText("42%")).toBeInTheDocument();
    // No sparkline or detail rendered
    expect(screen.queryByText(/./, { selector: "svg" })).toBeNull();
  });

  it("renders the detail line when provided", () => {
    render(<MiniMetric label="CPU" value="42%" detail="up from 30%" />);
    expect(screen.getByText("up from 30%")).toBeInTheDocument();
  });

  it("does not render a sparkline when trend is omitted", () => {
    const { container } = render(<MiniMetric label="CPU" value="42%" />);
    expect(container.querySelector("svg")).toBeNull();
  });

  it("does not render a sparkline when trend has only one point", () => {
    const { container } = render(<MiniMetric label="CPU" value="42%" trend={[50]} />);
    expect(container.querySelector("svg")).toBeNull();
  });

  it("renders a sparkline with the configured width/height when trend has 2+ points", () => {
    const { container } = render(<MiniMetric label="CPU" value="42%" trend={ascending} />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg).toHaveAttribute("width", "80");
    expect(svg).toHaveAttribute("height", "28");
    expect(svg).toHaveAttribute("viewBox", "0 0 80 28");
  });

  it("places the trend sparkline next to the text block", () => {
    const { container } = render(<MiniMetric label="RAM" value="8GB" trend={ascending} detail="steady" />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("flex");
    const textBlock = root.querySelector("div");
    expect(textBlock).not.toBeNull();
    expect(within(textBlock as HTMLElement).getByText("RAM")).toBeInTheDocument();
    expect(within(textBlock as HTMLElement).getByText("8GB")).toBeInTheDocument();
    expect(within(textBlock as HTMLElement).getByText("steady")).toBeInTheDocument();
    expect(root.querySelector("svg")).not.toBeNull();
  });
});