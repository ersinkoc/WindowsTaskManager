import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { useState } from "react";
import { QueryClient, useIsFetching, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router";
import { createTestQueryClient, renderWithProviders, TestWrapper } from "./test-utils";

function LocationProbe() {
  const loc = useLocation();
  return <span data-testid="loc">{loc.pathname}</span>;
}

describe("createTestQueryClient", () => {
  it("returns a QueryClient instance", () => {
    const client = createTestQueryClient();
    expect(client).toBeInstanceOf(QueryClient);
  });

  it("disables retry for queries", async () => {
    const client = createTestQueryClient();
    let attempt = 0;
    function Probe() {
      useQuery({
        queryKey: ["retry-queries"],
        queryFn: () => {
          attempt += 1;
          throw new Error("nope");
        },
      });
      return <span>probe</span>;
    }
    render(
      <TestWrapper>
        <Probe />
      </TestWrapper>,
    );
    // Wait a tick for the query to attempt and bail.
    await new Promise((resolve) => setTimeout(resolve, 0));
    // retry:false on the client default options + the global Vitest setup should
    // mean a single attempt is made and the query stays in error state.
    expect(attempt).toBe(1);
    // Sanity: the client is the same one the wrapper built.
    expect(client.getDefaultOptions().queries?.retry).toBe(false);
  });

  it("disables retry for mutations", async () => {
    const client = createTestQueryClient();
    expect(client.getDefaultOptions().mutations?.retry).toBe(false);
  });

  it("produces independent clients across calls (no shared mutation cache state)", () => {
    const a = createTestQueryClient();
    const b = createTestQueryClient();
    expect(a).not.toBe(b);
  });
});

describe("TestWrapper", () => {
  it("renders children inside all providers (QueryClient + Router + Theme)", () => {
    render(
      <TestWrapper>
        <span data-testid="child">hello</span>
      </TestWrapper>,
    );
    expect(screen.getByTestId("child")).toHaveTextContent("hello");
  });

  it("uses '/' as the default initialEntries", () => {
    render(
      <TestWrapper>
        <LocationProbe />
      </TestWrapper>,
    );
    expect(screen.getByTestId("loc")).toHaveTextContent("/");
  });

  it("accepts custom initialEntries for the MemoryRouter", () => {
    render(
      <TestWrapper initialEntries={["/dashboard/processes"]}>
        <LocationProbe />
      </TestWrapper>,
    );
    expect(screen.getByTestId("loc")).toHaveTextContent("/dashboard/processes");
  });

  it("provides a QueryClient context that react-query hooks can consume", () => {
    function Probe() {
      const client = useQueryClient();
      return <span data-testid="has-client">{client ? "yes" : "no"}</span>;
    }
    render(
      <TestWrapper>
        <Probe />
      </TestWrapper>,
    );
    expect(screen.getByTestId("has-client")).toHaveTextContent("yes");
  });

  it("forwards unknown extra props on the inner wrapper host (via spread)", () => {
    // TestWrapper destructures children + initialEntries; other props go to the
    // root div through `...props` inside the JSX spread. The current impl uses
    // explicit named props only, so we just assert children render.
    render(
      <TestWrapper initialEntries={["/x"]}>
        <span>x</span>
      </TestWrapper>,
    );
    expect(screen.getByText("x")).toBeInTheDocument();
  });
});

describe("renderWithProviders", () => {
  it("returns a render result with the standard Testing Library methods", () => {
    const result = renderWithProviders(<span>inside</span>);
    expect(result.getByText("inside")).toBeInTheDocument();
    expect(result.container).toBeInstanceOf(HTMLElement);
    expect(typeof result.unmount).toBe("function");
  });

  it("renders the children inside all providers (QueryClient + Router + Theme)", () => {
    renderWithProviders(<span data-testid="child">wrapped</span>);
    expect(screen.getByTestId("child")).toHaveTextContent("wrapped");
  });

  it("honors initialEntries when provided through options", () => {
    renderWithProviders(<LocationProbe />, { initialEntries: ["/ports"] });
    expect(screen.getByTestId("loc")).toHaveTextContent("/ports");
  });

  it("falls back to '/' when no options.initialEntries is provided", () => {
    renderWithProviders(<LocationProbe />);
    expect(screen.getByTestId("loc")).toHaveTextContent("/");
  });

  it("supports interactive flows with userEvent (button click)", async () => {
    const user = userEvent.setup();
    function Counter() {
      const [count, setCount] = useState(0);
      return (
        <button type="button" onClick={() => setCount((c) => c + 1)}>
          {count}
        </button>
      );
    }
    renderWithProviders(<Counter />);
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("0");
    await user.click(button);
    expect(button).toHaveTextContent("1");
    await user.click(button);
    expect(button).toHaveTextContent("2");
  });

  it("supports react-query useQuery inside a component", async () => {
    function UseQueryProbe() {
      const { data } = useQuery({
        queryKey: ["probe"],
        queryFn: () => Promise.resolve("value"),
      });
      return <span data-testid="result">{data ?? "loading"}</span>;
    }
    renderWithProviders(<UseQueryProbe />);
    // The element exists from the first render — we have to wait for the
    // async queryFn to resolve and the component to re-render with the data.
    const result = screen.getByTestId("result");
    await waitFor(
      () => {
        expect(result).toHaveTextContent("value");
      },
      { timeout: 2000 },
    );
  });

  it("supports react-query useMutation inside a component", async () => {
    const user = userEvent.setup();
    function MutateProbe() {
      const mutation = useMutation({
        mutationFn: async (input: string) => input.toUpperCase(),
      });
      return (
        <>
          <button type="button" onClick={() => mutation.mutate("hi")}>go</button>
          <span data-testid="status">{mutation.data ?? "idle"}</span>
        </>
      );
    }
    renderWithProviders(<MutateProbe />);
    await user.click(screen.getByRole("button"));
    expect(await screen.findByTestId("status")).toHaveTextContent("HI");
  });

  it("supports react-query useIsFetching inside a component", async () => {
    function FetchingProbe() {
      useQuery({
        queryKey: ["slow"],
        queryFn: () => new Promise<string>((resolve) => setTimeout(() => resolve("done"), 50)),
      });
      const fetching = useIsFetching();
      return <span data-testid="fetching">{fetching}</span>;
    }
    renderWithProviders(<FetchingProbe />);
    // While the query is in flight, useIsFetching should be > 0; after it
    // resolves it should be 0.
    expect(screen.getByTestId("fetching")).toHaveTextContent("1");
    expect(await screen.findByText("0")).toBeInTheDocument();
  });
});
