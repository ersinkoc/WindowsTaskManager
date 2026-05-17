import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { PortsPage } from "./ports-page";
import { testSnapshot } from "../test/fixtures";
import { TestWrapper } from "../test/test-utils";

const mockUseSystemSnapshotQuery = vi.fn();

vi.mock("../lib/api-client", () => ({
  useSystemSnapshotQuery: () => mockUseSystemSnapshotQuery(),
}));

describe("PortsPage", () => {
  beforeEach(() => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        ...testSnapshot,
        port_bindings: [
          {
            protocol: "tcp",
            local_addr: "127.0.0.1",
            local_port: 3000,
            remote_addr: "",
            remote_port: 0,
            state: "LISTEN",
            pid: 101,
            process: "chrome.exe",
            label: "Local dev",
          },
          {
            protocol: "udp",
            local_addr: "0.0.0.0",
            local_port: 5353,
            remote_addr: "224.0.0.251",
            remote_port: 5353,
            state: "",
            pid: 202,
            process: "mdns.exe",
            label: "mDNS",
          },
        ],
      },
      isLoading: false,
    });
  });

  it("filters bindings by protocol chip", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "TCP" }));

    await waitFor(() => {
      expect(screen.queryByText("mdns.exe")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("chrome.exe").length).toBeGreaterThan(0);
  });

  it("renders port list heading", () => {
    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByRole("heading", { name: "Full port list" })).toBeInTheDocument();
  });
});