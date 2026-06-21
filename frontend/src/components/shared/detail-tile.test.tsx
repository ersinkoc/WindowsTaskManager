import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DetailTile, SummaryCard } from "./detail-tile";

describe("DetailTile", () => {
  it("renders the label and value", () => {
    render(<DetailTile label="CPU" value="42%" />);
    expect(screen.getByText("CPU")).toBeInTheDocument();
    expect(screen.getByText("42%")).toBeInTheDocument();
  });

  it("uses the 'soft-panel' base class plus any custom className", () => {
    const { container } = render(
      <DetailTile label="Memory" value="8GB" className="extra-class" />,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("soft-panel");
    expect(root).toHaveClass("extra-class");
  });

  it("applies the valueClassName to the value line", () => {
    const { container } = render(
      <DetailTile label="Memory" value="8GB" valueClassName="text-error" />,
    );
    const root = container.firstElementChild as HTMLElement;
    // First child is the label, second child is the value.
    const valueEl = root.children[1] as HTMLElement;
    expect(valueEl).toHaveClass("text-error");
  });

  it("does not render the hint row when hint is omitted", () => {
    const { container } = render(<DetailTile label="CPU" value="42%" />);
    const root = container.firstElementChild as HTMLElement;
    // Only the label and value rows are rendered — no third child.
    expect(root.children).toHaveLength(2);
  });

  it("renders the hint when provided", () => {
    render(<DetailTile label="CPU" value="42%" hint="up from 30%" />);
    expect(screen.getByText("up from 30%")).toBeInTheDocument();
  });

  it("applies the secondary text class to the hint", () => {
    const { container } = render(<DetailTile label="CPU" value="42%" hint="up from 30%" />);
    const root = container.firstElementChild as HTMLElement;
    const hintEl = root.children[2] as HTMLElement;
    expect(hintEl).toHaveClass("text-secondary");
  });
});

describe("SummaryCard", () => {
  it("renders the label and value", () => {
    render(<SummaryCard label="CPU" value="42%" />);
    expect(screen.getByText("CPU")).toBeInTheDocument();
    expect(screen.getByText("42%")).toBeInTheDocument();
  });

  it("does not render the accent slot when accent is omitted", () => {
    const { container } = render(<SummaryCard label="CPU" value="42%" />);
    const root = container.firstElementChild as HTMLElement;
    // Card contains a flex row with the text block; the accent wrapper
    // (shrink-0 div) must not be present.
    expect(root.querySelector(".shrink-0")).toBeNull();
  });

  it("renders the accent node when provided", () => {
    render(
      <SummaryCard
        label="CPU"
        value="42%"
        accent={<span data-testid="badge">+5%</span>}
      />,
    );
    const accent = screen.getByTestId("badge");
    expect(accent).toBeInTheDocument();
    expect(accent).toHaveTextContent("+5%");
    // The accent lives inside the shrink-0 wrapper.
    const wrapper = accent.closest(".shrink-0");
    expect(wrapper).not.toBeNull();
  });

  it("applies the valueClassName to the value text", () => {
    const { container } = render(
      <SummaryCard label="CPU" value="42%" valueClassName="text-error" />,
    );
    const valueEl = container.querySelector(".text-error");
    expect(valueEl).not.toBeNull();
    expect(valueEl).toHaveTextContent("42%");
  });
});
