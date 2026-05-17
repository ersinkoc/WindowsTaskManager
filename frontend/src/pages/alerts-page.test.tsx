import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { AlertsPage } from "./alerts-page";
import { testAlerts } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

const mockUseAlertsQuery = vi.fn();
const mockUseAlertActionMutation = vi.fn();

vi.mock("../lib/api-client", () => ({
  useAlertsQuery: () => mockUseAlertsQuery(),
  useAlertActionMutation: () => mockUseAlertActionMutation(),
}));

describe("AlertsPage", () => {
  beforeEach(() => {
    mockUseAlertsQuery.mockReturnValue({
      data: testAlerts,
      isLoading: false,
    });
    mockUseAlertActionMutation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });
  });

  it("filters alerts by severity chip", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Critical" }));

    expect(screen.getByText("CPU spike detected")).toBeInTheDocument();
    expect(screen.queryByText("Memory growth observed")).not.toBeInTheDocument();
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
});
