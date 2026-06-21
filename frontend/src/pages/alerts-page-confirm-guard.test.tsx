import { render } from "@testing-library/react";
import { vi } from "vitest";
import { AlertsPage } from "./alerts-page";
import { testAlerts } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

// Capture every ConfirmDialog props object so we can invoke the latest onConfirm
// (the closure captures the current dismissCandidate from each render).
const dialogProps: Array<{
  open: boolean;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
}> = [];

const mockUseAlertsQuery = vi.fn();
const mockUseAlertActionMutation = vi.fn();

vi.mock("../lib/api-client", () => ({
  useAlertsQuery: () => mockUseAlertsQuery(),
  useAlertActionMutation: () => mockUseAlertActionMutation(),
}));

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();
  return { ...actual, useNavigate: () => vi.fn() };
});

vi.mock("../components/shared/confirm-dialog", () => ({
  ConfirmDialog: (props: { open: boolean; onConfirm: () => void; onOpenChange: (open: boolean) => void }) => {
    dialogProps.push(props);
    return null;
  },
}));

describe("AlertsPage dismiss guard", () => {
  beforeEach(() => {
    dialogProps.length = 0;
    mockUseAlertsQuery.mockReturnValue({ data: testAlerts, isLoading: false });
    mockUseAlertActionMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
  });

  it("onConfirm returns early when there is no dismiss candidate (defensive guard)", () => {
    render(<AlertsPage />, { wrapper: TestWrapper });

    // The very first render captured an onConfirm with dismissCandidate === null.
    // Calling it must be a no-op (the guard early-returns) and not throw.
    const firstConfirm = dialogProps[0]!;
    expect(() => firstConfirm.onConfirm()).not.toThrow();
  });
});
