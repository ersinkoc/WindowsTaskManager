import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { PortsPage } from "./ports-page";
import type { PortBinding } from "../types/api";
import { TestWrapper } from "../test/test-utils";

const mockUseSystemSnapshotQuery = vi.fn();
const mockNavigate = vi.fn();
const useNavigateMock = vi.fn(() => mockNavigate);

vi.mock("../lib/api-client", () => ({
  useSystemSnapshotQuery: () => mockUseSystemSnapshotQuery(),
}));

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();
  return {
    ...actual,
    useNavigate: () => useNavigateMock(),
  };
});

function binding(extra: Partial<PortBinding>): PortBinding {
  return {
    protocol: "tcp",
    local_addr: "127.0.0.1",
    local_port: 5000,
    remote_addr: "",
    remote_port: 0,
    state: "LISTEN",
    pid: 100,
    process: "svc.exe",
    label: "svc.exe",
    ...extra,
  };
}

// A spread exercising every filter, sort, and portLabel branch.
const sampleBindings: PortBinding[] = [
  // Known + dev + node: port 3000 (Vite/Node) on node.exe
  binding({ local_port: 3000, pid: 101, process: "node.exe", label: "node.exe" }),
  // Known: port 443 (HTTPS)
  binding({ local_port: 443, pid: 102, process: "nginx.exe", label: "nginx.exe" }),
  // UDP, not LISTEN, no known port — portLabel "Datagram"
  binding({ protocol: "udp", local_port: 5353, remote_addr: "224.0.0.251", remote_port: 5353, state: "", pid: 202, process: "mdns.exe", label: "mDNS" }),
  // TCP established, not LISTEN, has remote — portLabel "Active connection"
  binding({ local_port: 51000, remote_addr: "93.184.216.34", remote_port: 443, state: "ESTABLISHED", pid: 303, process: "chrome.exe", label: "Chrome" }),
];

describe("PortsPage", () => {
  beforeEach(() => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: { port_bindings: sampleBindings },
      isLoading: false,
    });
    mockNavigate.mockReset();
    useNavigateMock.mockReset();
    useNavigateMock.mockReturnValue(mockNavigate);
  });

  it("shows a skeleton while loading", () => {
    mockUseSystemSnapshotQuery.mockReturnValue({ data: undefined, isLoading: true });

    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.queryByRole("heading", { name: "Ports" })).not.toBeInTheDocument();
  });

  it("renders the empty state when there are no bindings", () => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: { port_bindings: [] },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("No port bindings yet")).toBeInTheDocument();
  });

  it("renders port list heading and summary metrics", () => {
    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByRole("heading", { name: "Full port list" })).toBeInTheDocument();
    expect(screen.getAllByText("Bindings").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Listening").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Processes").length).toBeGreaterThan(0);
  });

  it("filters bindings by the TCP protocol chip", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "TCP" }));

    await waitFor(() => {
      expect(screen.queryByText("mdns.exe")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("node.exe").length).toBeGreaterThan(0);
  });

  it("filters bindings by the UDP protocol chip", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "UDP" }));

    await waitFor(() => {
      expect(screen.queryByText("node.exe")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("mdns.exe").length).toBeGreaterThan(0);
  });

  it("filters bindings by the Listening chip", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "Listening" }));

    await waitFor(() => {
      expect(screen.queryByText("mdns.exe")).not.toBeInTheDocument();
      expect(screen.queryByText("Chrome")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("node.exe").length).toBeGreaterThan(0);
  });

  it("restores all bindings when clicking All after filtering", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getByRole("button", { name: "UDP" }));
    await waitFor(() => {
      expect(screen.queryByText("node.exe")).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "All" }));
    expect(screen.getAllByText("node.exe").length).toBeGreaterThan(0);
  });

  it("shows a filtered count when search narrows results", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search ports"), "chrome");

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /of 4 shown/ })).toBeInTheDocument();
    });
  });

  it("renders the Node.js servers section with instance count", () => {
    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByText(/server instance/)).toBeInTheDocument();
  });

  it("renders the Dev Ports section for non-standard dev ports", () => {
    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("Dev Ports")).toBeInTheDocument();
  });

  it("renders the Known ports section with app metadata", () => {
    render(<PortsPage />, { wrapper: TestWrapper });

    // 443 (HTTPS) is a well-known port; label appears in the known PortCell and the inspector.
    expect(screen.getByText("Known")).toBeInTheDocument();
    expect(screen.getAllByText("HTTPS").length).toBeGreaterThan(0);
  });

  it("opens the inspector drawer from a row Inspect button", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    // The "Inspect" button appears in the all-bindings table and the dev/known PortCells.
    const inspectButtons = screen.getAllByRole("button", { name: "Inspect" });
    await user.click(inspectButtons[0]!);

    expect(screen.getByText("Inspector")).toBeInTheDocument();
    // "Meaning" only appears inside the drawer DetailTile.
    expect(screen.getByText("Meaning")).toBeInTheDocument();
  });

  it("closes the inspector drawer via the close button", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    const inspectButtons = screen.getAllByRole("button", { name: "Inspect" });
    await user.click(inspectButtons[0]!);
    expect(screen.getByText("Inspector")).toBeInTheDocument();

    // The inspector header has an eyebrow "Inspector" then a heading; the close button is the
    // header's only Button. Walk from the eyebrow up to the flex header row and grab its button.
    const eyebrow = screen.getByText("Inspector", { selector: ".eyebrow" });
    const headerBar = eyebrow.parentElement?.parentElement;
    const closeButton = headerBar?.querySelector("button");
    if (!closeButton) throw new Error("inspector close button not found");
    await user.click(closeButton);
    expect(screen.queryByText("Inspector")).not.toBeInTheDocument();
  });

  it("closes the inspector drawer via the backdrop overlay", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);
    expect(screen.getByText("Inspector")).toBeInTheDocument();

    // The backdrop is the full-screen z-40 overlay div directly preceding the drawer panel.
    const eyebrow = screen.getByText("Inspector", { selector: ".eyebrow" });
    const drawerPanel = eyebrow.closest(".fixed");
    const overlay = drawerPanel?.previousElementSibling;
    if (!overlay) throw new Error("backdrop overlay not found");
    await user.click(overlay);
    expect(screen.queryByText("Inspector")).not.toBeInTheDocument();
  });

  it("navigates to processes when a PID cell is clicked in the table", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    // PID buttons in the table body.
    const pidButtons = screen.getAllByRole("button", { name: "101" });
    await user.click(pidButtons[0]!);

    expect(mockNavigate).toHaveBeenCalledWith("/processes?pid=101");
  });

  it("changes the sort key via the sort dropdown and exercises all keys", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    const select = screen.getByLabelText("Sort by");
    // Cycle through every sort option to cover each compareBindings branch.
    for (const value of ["process", "pid", "protocol", "local", "remote", "state"]) {
      await user.selectOptions(select, value);
      expect((select as HTMLSelectElement).value).toBe(value);
    }
  });

  it("toggles sort direction between asc and desc", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    // The toggle button is the sibling button after the sort select.
    const select = screen.getByLabelText("Sort by");
    const toggle = select.parentElement?.querySelectorAll("button");
    if (!toggle || toggle.length === 0) throw new Error("sort direction toggle not found");
    const directionToggle = toggle[toggle.length - 1]!;

    await user.click(directionToggle); // asc -> desc
    await user.click(directionToggle); // desc -> asc

    expect(screen.getByRole("heading", { name: "Full port list" })).toBeInTheDocument();
  });

  it("shows 'No port bindings yet' empty state does not leak when all filtered out", async () => {
    const user = userEvent.setup();
    render(<PortsPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Search ports"), "zzznomatch");

    await waitFor(() => {
      // Heading switches to the "0 of N shown" form.
      expect(screen.getByRole("heading", { name: /0 of 4 shown/ })).toBeInTheDocument();
    });
  });
});

// A second describe focused on portLabel branches that need the inspector open on
// non-well-known-port bindings (LISTEN, Datagram, Active connection, Open socket).
describe("PortsPage inspector portLabel", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    useNavigateMock.mockReset();
    useNavigateMock.mockReturnValue(mockNavigate);
  });

  it("shows 'Listening' for a non-known LISTEN port", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // Port 54321 is not a well-known port, state LISTEN → "Listening".
          binding({ local_port: 54321, pid: 411, process: "app.exe", label: "app.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);

    // "Listening" appears as the Meaning tile value; find it within the drawer.
    const meaningTile = screen.getByText("Meaning").parentElement;
    expect(meaningTile?.textContent).toContain("Listening");
  });

  it("shows 'Datagram' for a UDP, non-LISTEN, non-known binding", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // UDP, empty state, remote set but port not known → udp branch → "Datagram".
          binding({
            protocol: "udp",
            local_port: 54322,
            state: "",
            remote_addr: "10.0.0.1",
            remote_port: 53,
            pid: 412,
            process: "resolver.exe",
            label: "resolver.exe",
          }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);

    expect(screen.getByText("Datagram")).toBeInTheDocument();
  });

  it("shows 'Active connection' for a TCP binding with a remote peer", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // TCP, not LISTEN, non-known port, has remote → "Active connection".
          binding({
            local_port: 54323,
            state: "ESTABLISHED",
            remote_addr: "8.8.8.8",
            remote_port: 53,
            pid: 413,
            process: "client.exe",
            label: "client.exe",
          }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);

    expect(screen.getByText("Active connection")).toBeInTheDocument();
  });

  it("shows 'Open socket' for a TCP, non-LISTEN, no-remote, non-known binding", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // TCP, not LISTEN, no remote, non-known port, empty state → "Open socket".
          binding({
            local_port: 54324,
            state: "",
            remote_addr: "",
            remote_port: 0,
            pid: 414,
            process: "idle.exe",
            label: "idle.exe",
          }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);

    expect(screen.getByText("Open socket")).toBeInTheDocument();
  });

  it("renders multiple node server instances across distinct ports", () => {
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          binding({ local_port: 3000, pid: 101, process: "node.exe", label: "node.exe" }),
          binding({ local_port: 8000, pid: 102, process: "node.exe", label: "node.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // Plural form when more than one instance.
    expect(screen.getByText(/2 server instances/)).toBeInTheDocument();
  });

  it("uses process, then label, then em-dash when process is empty", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // process empty → falls back to label in the table row and inspector.
          binding({ local_port: 54325, pid: 415, process: "", label: "fallback-label", state: "LISTEN" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // "fallback-label" appears in the table, mobile card, dev PortCell, and inspector.
    expect(screen.getAllByText("fallback-label").length).toBeGreaterThan(0);
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);
    // Inspector Process tile also falls back to label.
    const meaningTile = screen.getByText("Meaning");
    const drawerBody = meaningTile.closest(".flex-1");
    const metricLabels = drawerBody?.querySelectorAll(".metric-label");
    const processLabel = Array.from(metricLabels ?? []).find((el) => el.textContent === "Process");
    expect(processLabel?.parentElement?.textContent).toContain("fallback-label");
  });

  it("opens the inspector when a PortCell (node/dev/known tile) is clicked", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          binding({ local_port: 3000, pid: 101, process: "node.exe", label: "node.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // PortCells render as buttons containing the port number "3000" (mono bold).
    // Click the Node.js PortCell — it carries the port number text.
    const portCells = screen.getAllByRole("button").filter((b) => b.textContent?.includes("3000") && b.textContent?.includes("Node"));
    // The PortCell itself is a button; find the one whose details open the drawer.
    await user.click(portCells[0]!);

    expect(screen.getByText("Inspector")).toBeInTheDocument();
  });

  it("opens the inspector from a Dev Ports PortCell", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // Dev port 8888 (Jupyter) but not a node process → only dev section, not node section.
          binding({ local_port: 8888, pid: 101, process: "jupyter.exe", label: "jupyter.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("Dev Ports")).toBeInTheDocument();
    // The dev PortCell is a button whose content includes the port 8888 and the Jupyter app label.
    const devCell = screen.getAllByRole("button").filter((b) => b.textContent?.includes("8888"));
    await user.click(devCell[0]!);

    expect(screen.getByText("Inspector")).toBeInTheDocument();
  });

  it("opens the inspector from a Known ports PortCell", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // Known port 3306 (MySQL), not a dev port → only known section.
          binding({ local_port: 3306, pid: 102, process: "mysqld.exe", label: "mysqld.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("Known")).toBeInTheDocument();
    // The known PortCell is a button whose content includes port 3306 and the MySQL label.
    const knownCell = screen.getAllByRole("button").filter((b) => b.textContent?.includes("3306"));
    await user.click(knownCell[0]!);

    expect(screen.getByText("Inspector")).toBeInTheDocument();
  });

  it("opens the inspector from the desktop table Inspect button", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          binding({ local_port: 3000, pid: 101, process: "node.exe", label: "node.exe" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // Desktop table Inspect buttons live inside a <table>. Find the table, then its Inspect button.
    const table = document.querySelector("table");
    const tableInspect = Array.from(table?.querySelectorAll("button") ?? []).find(
      (b) => b.textContent === "Inspect",
    );
    if (!tableInspect) throw new Error("desktop table Inspect button not found");
    await user.click(tableInspect);

    expect(screen.getByText("Inspector")).toBeInTheDocument();
  });

  it("falls back to em-dash and Unknown when both process and label are empty", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // Both empty → table row and cards render the em-dash fallback.
          binding({ local_port: 54326, pid: 416, process: "", label: "", state: "LISTEN" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // The em-dash fallback renders in the table and mobile card process cell.
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);

    // Open the inspector on this binding — the header and Process tile show "Unknown".
    await user.click(screen.getAllByRole("button", { name: "Inspect" })[0]!);
    expect(screen.getAllByText("Unknown").length).toBeGreaterThan(0);
  });

  it("sorts by process key using label fallback when process is empty", async () => {
    const user = userEvent.setup();
    mockUseSystemSnapshotQuery.mockReturnValue({
      data: {
        port_bindings: [
          // Several bindings, some with empty process → forces label fallback on both
          // sides of the comparison during sort.
          binding({ local_port: 54327, pid: 417, process: "", label: "zzz-label", state: "LISTEN" }),
          binding({ local_port: 54328, pid: 418, process: "aaa.exe", label: "aaa.exe", state: "LISTEN" }),
          binding({ local_port: 54329, pid: 419, process: "", label: "mmm-label", state: "LISTEN" }),
        ],
      },
      isLoading: false,
    });

    render(<PortsPage />, { wrapper: TestWrapper });

    // Sort by "process" exercises (left.process || left.label) and (right.process || right.label).
    await user.selectOptions(screen.getByLabelText("Sort by"), "process");

    expect(screen.getByRole("heading", { name: "Full port list" })).toBeInTheDocument();
  });
});
