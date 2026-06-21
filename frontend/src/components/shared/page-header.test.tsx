import { render, screen } from "@testing-library/react";
import { Activity } from "lucide-react";
import { describe, expect, it } from "vitest";
import { PageHeader } from "./page-header";

describe("PageHeader", () => {
  it("renders the title and description", () => {
    render(<PageHeader title="Dashboard" description="Live system view" />);
    expect(screen.getByRole("heading", { level: 1, name: "Dashboard" })).toBeInTheDocument();
    expect(screen.getByText("Live system view")).toBeInTheDocument();
  });

  it("uses 'Live workspace' as the default eyebrow", () => {
    render(<PageHeader title="Dashboard" description="d" />);
    expect(screen.getByText("Live workspace")).toBeInTheDocument();
  });

  it("renders a custom eyebrow when provided", () => {
    render(<PageHeader title="Dashboard" description="d" eyebrow="Snapshot view" />);
    expect(screen.getByText("Snapshot view")).toBeInTheDocument();
    expect(screen.queryByText("Live workspace")).toBeNull();
  });

  it("renders the default Sparkles icon in the eyebrow row", () => {
    render(<PageHeader title="t" description="d" />);
    // Sparkles is a lucide icon — renders an <svg>. The eyebrow row contains
    // exactly one icon (the default Sparkles).
    const eyebrow = screen.getByText("Live workspace").parentElement as HTMLElement;
    const icons = eyebrow.querySelectorAll("svg");
    expect(icons).toHaveLength(1);
    expect(icons[0]).toHaveClass("h-3.5", "w-3.5", "text-accent");
  });

  it("renders a custom icon when provided via the icon prop", () => {
    render(
      <PageHeader
        title="t"
        description="d"
        icon={Activity}
        eyebrow="Activity"
      />,
    );
    const eyebrow = screen.getByText("Activity").parentElement as HTMLElement;
    const icons = eyebrow.querySelectorAll("svg");
    expect(icons).toHaveLength(1);
    // Activity's path uses an "activity" lucide component (data-* attribute set by lucide-react).
    // The icon must still receive the configured h-3.5/w-3.5/text-accent classes.
    expect(icons[0]).toHaveClass("h-3.5", "w-3.5", "text-accent");
  });

  it("does not render the meta row when meta is omitted", () => {
    const { container } = render(<PageHeader title="t" description="d" />);
    // The hero-panel structure is section > div > div(text) + (optional actions div).
    const textBlock = container.querySelector(".min-w-0.max-w-3xl") as HTMLElement;
    // Only the eyebrow, h1, and description — no extra <div> for meta.
    expect(textBlock).not.toBeNull();
    const metaWrapper = textBlock.querySelector("div.mt-2.flex");
    expect(metaWrapper).toBeNull();
  });

  it("renders the meta row when meta is provided", () => {
    render(
      <PageHeader
        title="t"
        description="d"
        meta={<span data-testid="meta-item">live</span>}
      />,
    );
    const metaItem = screen.getByTestId("meta-item");
    expect(metaItem).toBeInTheDocument();
    const metaRow = metaItem.parentElement as HTMLElement;
    expect(metaRow).toHaveClass("mt-2", "flex", "flex-wrap", "items-center", "gap-1.5");
  });

  it("does not render the actions slot when actions is omitted", () => {
    const { container } = render(<PageHeader title="t" description="d" />);
    // The actions wrapper has the "relative z-10 flex w-full ..." classes.
    const actions = container.querySelector(".relative.z-10.flex.w-full");
    expect(actions).toBeNull();
  });

  it("renders the actions slot when actions is provided", () => {
    render(
      <PageHeader
        title="t"
        description="d"
        actions={<button type="button">Export</button>}
      />,
    );
    expect(screen.getByRole("button", { name: "Export" })).toBeInTheDocument();
  });

  it("applies the responsive classes to the actions wrapper", () => {
    const { container } = render(
      <PageHeader
        title="t"
        description="d"
        actions={<button type="button">A</button>}
      />,
    );
    const actionsWrapper = container.querySelector(".relative.z-10.flex.w-full") as HTMLElement;
    expect(actionsWrapper).not.toBeNull();
    expect(actionsWrapper).toHaveClass("flex-col", "sm:flex-row");
    expect(actionsWrapper).toHaveClass("border-t", "border-border", "pt-3");
  });

  it("renders both meta and actions side by side when both are provided", () => {
    render(
      <PageHeader
        title="t"
        description="d"
        meta={<span data-testid="m">m</span>}
        actions={<button type="button">act</button>}
      />,
    );
    expect(screen.getByTestId("m")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "act" })).toBeInTheDocument();
  });

  it("uses the 'hero-panel' wrapper", () => {
    const { container } = render(<PageHeader title="t" description="d" />);
    const hero = container.querySelector("section.hero-panel");
    expect(hero).not.toBeNull();
  });
});
