import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { SearchInput } from "./search-input";

type SearchInputProps = React.ComponentProps<typeof SearchInput>;

function ControlledSearchInput({
  initial = "",
  onChange,
  ...rest
}: Omit<SearchInputProps, "value" | "onChange"> & {
  initial?: string;
  onChange?: (v: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <SearchInput
      {...rest}
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange?.(next);
      }}
    />
  );
}

describe("SearchInput", () => {
  it("renders the search input with the given aria-label and placeholder", () => {
    render(
      <SearchInput
        ariaLabel="Search processes"
        placeholder="Search by name"
        value=""
        onChange={() => undefined}
      />,
    );
    const input = screen.getByLabelText("Search processes");
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute("placeholder", "Search by name");
  });

  it("uses sm:w-72 as the default widthClassName", () => {
    const { container } = render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value=""
        onChange={() => undefined}
      />,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("sm:w-72");
  });

  it("applies a custom widthClassName when provided", () => {
    const { container } = render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value=""
        widthClassName="sm:w-96"
        onChange={() => undefined}
      />,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("sm:w-96");
    expect(root).not.toHaveClass("sm:w-72");
  });

  it("reflects the controlled value in the input", () => {
    render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value="chrome"
        onChange={() => undefined}
      />,
    );
    expect(screen.getByLabelText("Search")).toHaveValue("chrome");
  });

  it("calls onChange with the typed value", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ControlledSearchInput ariaLabel="Search" placeholder="q" onChange={handleChange} />,
    );
    const input = screen.getByLabelText("Search");
    await user.type(input, "abc");
    expect(handleChange).toHaveBeenCalledWith("a");
    expect(handleChange).toHaveBeenCalledWith("ab");
    expect(handleChange).toHaveBeenCalledWith("abc");
  });

  it("does not render a clear button when value is empty", () => {
    render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value=""
        onChange={() => undefined}
      />,
    );
    expect(screen.queryByRole("button", { name: /clear search/i })).toBeNull();
  });

  it("renders a clear button when value is non-empty", () => {
    render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value="hello"
        onChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Clear Search" })).toBeInTheDocument();
  });

  it("uses the ariaLabel in the clear button's accessible name", () => {
    render(
      <SearchInput
        ariaLabel="Search alerts"
        placeholder="q"
        value="x"
        onChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Clear Search alerts" })).toBeInTheDocument();
  });

  it("calls onChange with an empty string when the clear button is clicked", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ControlledSearchInput
        ariaLabel="Search"
        placeholder="q"
        initial="abc"
        onChange={handleChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Clear Search" }));
    expect(handleChange).toHaveBeenLastCalledWith("");
  });

  it("renders the search icon and clear icon as svg elements", () => {
    const { container } = render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value="abc"
        onChange={() => undefined}
      />,
    );
    // Two SVGs: the search icon (always) and the X icon (when value is non-empty).
    expect(container.querySelectorAll("svg")).toHaveLength(2);
  });

  it("renders only the search icon (no X) when the value is empty", () => {
    const { container } = render(
      <SearchInput
        ariaLabel="Search"
        placeholder="q"
        value=""
        onChange={() => undefined}
      />,
    );
    expect(container.querySelectorAll("svg")).toHaveLength(1);
  });
});
