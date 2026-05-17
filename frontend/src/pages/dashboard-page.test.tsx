import { render, screen, within } from "@testing-library/react";
import { vi } from "vitest";
import { DashboardPage } from "./dashboard-page";
import { testAlerts, testSnapshot } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

const mockUseSystemSnapshotQuery = vi.fn();
const mockUseAlertsQuery = vi.fn();

vi.mock("../lib/api-client", () => ({
  useSystemSnapshotQuery: () => mockUseSystemSnapshotQuery(),
  useAlertsQuery: () => mockUseAlertsQuery(),
}));

describe("DashboardPage", () => {
  beforeEach(() => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        ...testSnapshot,
        processes: [
          ...testSnapshot.processes,
          {
            pid: 202,
            name: "node.exe",
            cpu_percent: 83.4,
            working_set: 900_000_000,
            thread_count: 21,
            connections: 3,
            is_critical: false,
          },
        ],
      },
      isLoading: false,
    });
    mockUseAlertsQuery.mockReturnValue({
      data: testAlerts,
    });
  });

  it("shows the highest CPU processes first in the top processes table", () => {
    render(<DashboardPage />, { wrapper: TestWrapper });

    const table = screen.getByRole("table");
    const rows = within(table).getAllByRole("row");

    expect(rows[1]).toHaveTextContent(/^202node\.exe83\.4%/);
    expect(rows[2]).toHaveTextContent(/^101chrome\.exe21\.7%/);
  });
});
