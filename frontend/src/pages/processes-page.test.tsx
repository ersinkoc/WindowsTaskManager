import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { ProcessesPage } from "./processes-page";
import { testSnapshot } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

const mockUseSystemSnapshotQuery = vi.fn();
const mockUseInfoQuery = vi.fn();
const mockUseProcessActionMutation = vi.fn();
const mockUseProcessConnectionsQuery = vi.fn();

vi.mock("../lib/api-client", () => ({
  useSystemSnapshotQuery: () => mockUseSystemSnapshotQuery(),
  useInfoQuery: () => mockUseInfoQuery(),
  useProcessActionMutation: () => mockUseProcessActionMutation(),
  useProcessConnectionsQuery: () => mockUseProcessConnectionsQuery(),
}));

describe("ProcessesPage", () => {
  beforeEach(() => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: testSnapshot,
      isLoading: false,
    });
    mockUseInfoQuery.mockReturnValue({
      data: {
        self_pid: 999,
      },
    });
    mockUseProcessActionMutation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });
    mockUseProcessConnectionsQuery.mockReturnValue({
      data: [],
      isLoading: false,
    });
  });

  it("filters processes by search text", async () => {
    const user = userEvent.setup();
    render(<ProcessesPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search processes by name or PID"), "chrome");

    await waitFor(() => {
      expect(screen.queryByText(/^System$/)).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("chrome.exe").length).toBeGreaterThan(0);
  });

  it("opens a confirmation dialog before killing a process", async () => {
    const user = userEvent.setup();
    render(<ProcessesPage />, { wrapper: TestWrapper });

    await user.click(screen.getAllByRole("button", { name: "Kill" })[0]!);

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText("Kill chrome.exe?")).toBeInTheDocument();
  });

  it("sorts rows when the header is clicked", async () => {
    const user = userEvent.setup();
    render(<ProcessesPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Sort by Name" }));

    const rows = screen.getAllByRole("row");
    expect(rows[1]).toHaveTextContent(/^4System/);

    await user.click(screen.getByRole("button", { name: "Sort by Name" }));

    const updatedRows = screen.getAllByRole("row");
    expect(updatedRows[1]).toHaveTextContent(/^101chrome\.exe/);
  });

  it("prefers live port bindings over stale process connection counts", () => {
    render(<ProcessesPage />, { wrapper: TestWrapper });

    expect(screen.getAllByText("2").length).toBeGreaterThan(0);
  });
});
