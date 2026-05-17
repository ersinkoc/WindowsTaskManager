import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { RulesPage } from "./rules-page";
import { TestWrapper } from "../test/test-utils";

const mockUseRulesQuery = vi.fn();
const mockUseRulesUpdateMutation = vi.fn();

vi.mock("../lib/api-client", () => ({
  useRulesQuery: () => mockUseRulesQuery(),
  useRulesUpdateMutation: () => mockUseRulesUpdateMutation(),
}));

describe("RulesPage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    mockUseRulesQuery.mockReturnValue({
      data: {
        rules: [
          {
            name: "High CPU chrome",
            enabled: true,
            match: "chrome.exe",
            metric: "cpu_percent",
            op: ">=",
            threshold: 80,
            for_seconds: 30,
            action: "alert",
            cooldown_seconds: 300,
          },
        ],
      },
      isLoading: false,
    });
    mockUseRulesUpdateMutation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
    });
  });

  it("renders add rule controls even when there are no existing rules", () => {
    mockUseRulesQuery.mockReturnValue({
      data: { rules: [] },
      isLoading: false,
    });

    render(<RulesPage />, { wrapper: TestWrapper });

    expect(screen.getByRole("heading", { name: "Add rule" })).toBeInTheDocument();
    expect(screen.getByLabelText("Rule name")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add rule" })).toBeInTheDocument();
  });

  it("adds a new rule through the backend update mutation", async () => {
    const user = userEvent.setup();
    const mutate = vi.fn();
    mockUseRulesUpdateMutation.mockReturnValue({
      isPending: false,
      mutate,
    });

    render(<RulesPage />, { wrapper: TestWrapper });

    await user.type(screen.getByLabelText("Rule name"), "Memory leak guard");
    await user.type(screen.getByLabelText("Rule match"), "claude.exe");
    await user.clear(screen.getByLabelText("Rule threshold"));
    await user.type(screen.getByLabelText("Rule threshold"), "2147483648");
    await user.selectOptions(screen.getByLabelText("Rule metric"), "memory_bytes");
    await user.click(screen.getByRole("button", { name: "Add rule" }));

    expect(mutate).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({
          name: "Memory leak guard",
          match: "claude.exe",
          metric: "memory_bytes",
          threshold: 2147483648,
        }),
      ]),
      expect.any(Object),
    );
  });

  it("opens existing rules in edit mode and saves the updated rule", async () => {
    const user = userEvent.setup();
    const mutate = vi.fn();
    mockUseRulesUpdateMutation.mockReturnValue({
      isPending: false,
      mutate,
    });

    render(<RulesPage />);

    await user.click(screen.getByRole("button", { name: "Edit" }));
    const ruleNameInputs = screen.getAllByLabelText("Rule name");
    const editingInput = ruleNameInputs[1];
    expect(editingInput).toBeDefined();
    if (!editingInput) {
      throw new Error("Editing rule input not found");
    }
    await user.clear(editingInput);
    await user.type(editingInput, "High CPU chrome updated");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(mutate).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({
          name: "High CPU chrome updated",
        }),
      ]),
      expect.any(Object),
    );
  });

  it("prefills the add-rule form from queued AI suggestion state", () => {
    window.sessionStorage.setItem(
      "wtm:rule-draft-prefill",
      JSON.stringify({
        name: "AI memory guard",
        match: "claude.exe",
        metric: "memory_bytes",
        threshold: 2147483648,
      }),
    );

    render(<RulesPage />);

    expect(screen.getAllByLabelText("Rule name")[0]).toHaveValue("AI memory guard");
    expect(screen.getByLabelText("Rule match")).toHaveValue("claude.exe");
    expect(screen.getByLabelText("Rule metric")).toHaveValue("memory_bytes");
    expect(window.sessionStorage.getItem("wtm:rule-draft-prefill")).toBeNull();
  });

  it("uses a substring-based match in the thread storm template", async () => {
    const user = userEvent.setup();
    render(<RulesPage />);

    await user.click(screen.getByRole("button", { name: /Thread storm/ }));

    expect(screen.getByLabelText("Rule match")).toHaveValue(".exe");
  });

  it("asks for confirmation before deleting a rule", async () => {
    const user = userEvent.setup();
    const mutate = vi.fn();
    mockUseRulesUpdateMutation.mockReturnValue({
      isPending: false,
      mutate,
    });

    render(<RulesPage />);

    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Delete rule" }));

    expect(mutate).toHaveBeenCalledWith(
      [],
      expect.any(Object),
    );
  });
});
