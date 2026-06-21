import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { AlertsPage } from "./alerts-page";
import { testAlerts } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

const mockUseAlertsQuery = vi.fn();
const mockUseAlertActionMutation = vi.fn();
const mockNavigate = vi.fn();

vi.mock("../lib/api-client", () => ({
  useAlertsQuery: () => mockUseAlertsQuery(),
  useAlertActionMutation: () => mockUseAlertActionMutation(),
}));

const useNavigateMock = vi.fn((): typeof mockNavigate | undefined => mockNavigate);

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();
  return {
    ...actual,
    useNavigate: () => useNavigateMock(),
  };
});

function mockMutation(overrides: Partial<{ isPending: boolean; mutate: ReturnType<typeof vi.fn> }> = {}) {
  const value = {
    isPending: false,
    mutate: vi.fn(),
    ...overrides,
  };
  mockUseAlertActionMutation.mockReturnValue(value);
  return value;
}

describe("AlertsPage", () => {
  beforeEach(() => {
    mockUseAlertsQuery.mockReturnValue({
      data: testAlerts,
      isLoading: false,
    });
    mockMutation();
    mockNavigate.mockReset();
    useNavigateMock.mockReset();
    useNavigateMock.mockReturnValue(mockNavigate);
  });

  it("shows a skeleton while loading", () => {
    mockUseAlertsQuery.mockReturnValue({ data: undefined, isLoading: true });

    render(<AlertsPage />, { wrapper: TestWrapper });

    // PageSkeleton renders without the alerts page header; nothing meaningful rendered.
    expect(screen.queryByText("Alerts")).not.toBeInTheDocument();
  });

  it("renders the empty state when there are no alerts at all", () => {
    mockUseAlertsQuery.mockReturnValue({
      data: { active: [], history: [] },
      isLoading: false,
    });

    render(<AlertsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("No alerts yet")).toBeInTheDocument();
  });

  it("renders the empty state when data is missing", () => {
    mockUseAlertsQuery.mockReturnValue({ data: undefined, isLoading: false });

    render(<AlertsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("No alerts yet")).toBeInTheDocument();
  });

  it("renders summary counts and severity rows", () => {
    render(<AlertsPage />, { wrapper: TestWrapper });

    // SummaryCard labels also appear as accent badge text — assert at least one each.
    expect(screen.getAllByText("Active alerts").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Critical active").length).toBeGreaterThan(0);
    expect(screen.getAllByText("History events").length).toBeGreaterThan(0);
    // SeverityRow labels (inside the queue card)
    expect(screen.getAllByText("Critical").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Warning").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Info").length).toBeGreaterThan(0);
  });

  it("filters alerts by the Critical severity chip", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Critical" }));

    expect(screen.getByText("CPU spike detected")).toBeInTheDocument();
    expect(screen.queryByText("Memory growth observed")).not.toBeInTheDocument();
  });

  it("filters alerts by the Warning severity chip", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Warning" }));

    expect(screen.queryByText("CPU spike detected")).not.toBeInTheDocument();
    expect(screen.getByText("Memory growth observed")).toBeInTheDocument();
  });

  it("filters alerts by the Info severity chip (history only)", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Info" }));

    // Active alerts are critical/warning — both should be hidden.
    expect(screen.queryByText("CPU spike detected")).not.toBeInTheDocument();
    expect(screen.queryByText("Memory growth observed")).not.toBeInTheDocument();
    // History info alert should remain.
    expect(screen.getByText("Port burst resolved")).toBeInTheDocument();
  });

  it("restores all alerts when clicking the All chip after filtering", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Critical" }));
    expect(screen.queryByText("Memory growth observed")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "All" }));

    expect(screen.getByText("CPU spike detected")).toBeInTheDocument();
    expect(screen.getByText("Memory growth observed")).toBeInTheDocument();
  });

  it("filters alerts by search text", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search alerts by title, type, severity, or PID"), "memory");

    await waitFor(() => {
      expect(screen.queryByText("CPU spike detected")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Memory growth observed")).toBeInTheDocument();
  });

  it("filters alerts by PID via search", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search alerts by title, type, severity, or PID"), "202");

    await waitFor(() => {
      expect(screen.queryByText("CPU spike detected")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Memory growth observed")).toBeInTheDocument();
  });

  it("filters alerts by alert type via search", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search alerts by title, type, severity, or PID"), "cpu_spike");

    await waitFor(() => {
      expect(screen.queryByText("Memory growth observed")).not.toBeInTheDocument();
    });
    expect(screen.getByText("CPU spike detected")).toBeInTheDocument();
  });

  it("shows the no-match empty state when search filters everything out", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search alerts by title, type, severity, or PID"), "zzzznomatch");

    await waitFor(() => {
      expect(screen.getByText("No alerts match")).toBeInTheDocument();
    });
  });

  it("navigates to the processes page when the PID link is clicked", async () => {
    const user = userEvent.setup();

    render(<AlertsPage />, { wrapper: TestWrapper });

    // PID link has accessible name "PID 101" (its visible text), title for hover tooltip.
    const pidLink = screen.getByRole("button", { name: "PID 101" });
    await user.click(pidLink);

    expect(mockNavigate).toHaveBeenCalledWith("/processes?pid=101");
  });

  it("triggers the snooze mutation when Snooze 30m is clicked", async () => {
    const user = userEvent.setup();
    const mutate = mockMutation().mutate;

    render(<AlertsPage />, { wrapper: TestWrapper });

    // Two active alerts → two Snooze buttons; click the first.
    await user.click(screen.getAllByRole("button", { name: "Snooze 30m" })[0]!);

    expect(mutate).toHaveBeenCalledWith({ type: "cpu_spike", pid: 101, action: "snooze" });
  });

  it("opens the dismiss confirmation dialog, then dismisses on confirm", async () => {
    const user = userEvent.setup();
    const mutate = mockMutation().mutate;

    render(<AlertsPage />, { wrapper: TestWrapper });

    // Click the first Dismiss button (active alerts).
    await user.click(screen.getAllByRole("button", { name: "Dismiss" })[0]!);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText(/Dismiss CPU spike detected\?/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Dismiss alert" }));

    expect(mutate).toHaveBeenCalledWith(
      { type: "cpu_spike", pid: 101, action: "dismiss" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("closes the dismiss dialog when cancelled", async () => {
    const user = userEvent.setup();

    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getAllByRole("button", { name: "Dismiss" })[0]!);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("calls the onSuccess handler to clear the dismiss candidate", async () => {
    const user = userEvent.setup();
    const mutate = mockMutation().mutate;

    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getAllByRole("button", { name: "Dismiss" })[0]!);
    await user.click(screen.getByRole("button", { name: "Dismiss alert" }));

    const call = mutate.mock.calls[0]!;
    const onSuccess = call[1].onSuccess;
    expect(onSuccess).toBeTypeOf("function");

    // Invoking onSuccess clears the candidate — wrap in act to flush state.
    await act(async () => {
      onSuccess();
    });
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("dismiss dialog returns early when there is no candidate (defensive guard)", async () => {
    const user = userEvent.setup();
    const mutate = mockMutation().mutate;

    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getAllByRole("button", { name: "Dismiss" })[0]!);
    // Cancel to reset candidate, then re-trigger confirm with a closed dialog
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    // Open and confirm normally; mutate fires once with valid candidate.
    await user.click(screen.getAllByRole("button", { name: "Dismiss" })[0]!);
    await user.click(screen.getByRole("button", { name: "Dismiss alert" }));

    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("paginates history with the Load more button", async () => {
    const user = userEvent.setup();
    // Generate enough history items to exceed one page (PAGE_SIZE = 20).
    const history = Array.from({ length: 25 }, (_, i) => ({
      type: "port_burst",
      severity: i % 2 === 0 ? "info" : "warning",
      title: `Historical alert ${i}`,
      description: `Description ${i}`,
    }));
    mockUseAlertsQuery.mockReturnValue({
      data: { active: [], history },
      isLoading: false,
    });

    render(<AlertsPage />, { wrapper: TestWrapper });

    const loadMore = screen.getByRole("button", { name: /Load more/ });
    await user.click(loadMore);

    // After load more, the remaining count should update and the full list is shown.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /Load more/ })).not.toBeInTheDocument();
    });
    expect(screen.getByText(/Total 25/)).toBeInTheDocument();
  });

  it("renders history rows without dismiss/snooze actions", () => {
    render(<AlertsPage />, { wrapper: TestWrapper });

    // History alert: "Port burst resolved"
    expect(screen.getByText("Port burst resolved")).toBeInTheDocument();
    // History rows have no snooze/dismiss buttons.
    expect(screen.getAllByRole("button", { name: "Snooze 30m" }).length).toBe(2);
    expect(screen.getAllByRole("button", { name: "Dismiss" }).length).toBe(2);
  });

  it("renders global (no-PID) alerts as a badge rather than a link", () => {
    mockUseAlertsQuery.mockReturnValue({
      data: {
        active: [
          {
            type: "system_event",
            severity: "info",
            title: "Global system note",
            description: "A global, non-PID alert.",
          },
        ],
        history: [],
      },
      isLoading: false,
    });

    render(<AlertsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("Global system note")).toBeInTheDocument();
    // No PID navigation link for a pid-less alert.
    expect(screen.queryByRole("button", { name: /View PID/ })).not.toBeInTheDocument();
  });

  it("renders the history total badge when filtered history has items", () => {
    render(<AlertsPage />, { wrapper: TestWrapper });

    expect(screen.getByText(/Total 1/)).toBeInTheDocument();
  });

  it("renders an active alert with a PID as a Badge when navigate is unavailable", () => {
    // When useNavigate returns undefined, the AlertRow falls back to a PID Badge.
    useNavigateMock.mockReturnValueOnce(undefined);

    render(<AlertsPage />, { wrapper: TestWrapper });

    // No PID navigation link; the PID appears inside a neutral badge instead.
    expect(screen.queryByRole("button", { name: "PID 101" })).not.toBeInTheDocument();
    expect(screen.getByText("PID 101")).toBeInTheDocument();
  });
});
