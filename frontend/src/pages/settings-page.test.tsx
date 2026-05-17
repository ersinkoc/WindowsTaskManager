import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { SettingsPage } from "./settings-page";
import { TestWrapper } from "../test/test-utils";

const mockUseAIConfigQuery = vi.fn();
const mockUseAIPresetsQuery = vi.fn();
const mockUseTelegramConfigQuery = vi.fn();
const mockUseConfigQuery = vi.fn();
const mockUseAIConfigMutation = vi.fn();
const mockUseTelegramConfigMutation = vi.fn();
const mockUseConfigUpdateMutation = vi.fn();
const mockUseTelegramTestMutation = vi.fn();
const mockUseAITestMutation = vi.fn();

vi.mock("../lib/api-client", () => ({
  useAIConfigQuery: () => mockUseAIConfigQuery(),
  useAIPresetsQuery: () => mockUseAIPresetsQuery(),
  useTelegramConfigQuery: () => mockUseTelegramConfigQuery(),
  useConfigQuery: () => mockUseConfigQuery(),
  useAIConfigMutation: () => mockUseAIConfigMutation(),
  useTelegramConfigMutation: () => mockUseTelegramConfigMutation(),
  useConfigUpdateMutation: () => mockUseConfigUpdateMutation(),
  useTelegramTestMutation: () => mockUseTelegramTestMutation(),
  useAITestMutation: () => mockUseAITestMutation(),
}));

describe("SettingsPage", () => {
  beforeEach(() => {
    mockUseAIConfigQuery.mockReturnValue({
      data: {
        enabled: true,
        provider: "openai",
        api_key: "****abcd",
        model: "gpt-5-mini",
        endpoint: "https://api.openai.com/v1/chat/completions",
        extra_headers: {},
        language: "en",
        max_tokens: 1024,
        max_requests_per_minute: 5,
        include_process_tree: true,
        include_port_map: true,
      },
      isLoading: false,
    });
    mockUseAIPresetsQuery.mockReturnValue({
      data: [{ id: "openai", label: "OpenAI GPT", provider: "openai", endpoint: "https://api.openai.com/v1/chat/completions", model: "gpt-5-mini", api_key_hint: "sk-..." }],
    });
    mockUseTelegramConfigQuery.mockReturnValue({
      data: {
        enabled: false,
        bot_token: "",
        allowed_chat_ids: [],
        api_base_url: "https://api.telegram.org",
        poll_timeout_sec: 25,
        notify_on_critical: true,
        notification_mode: "high_value",
        notification_types: ["runaway_cpu", "memory_leak", "rule:*"],
        require_confirm: true,
        confirm_ttl_sec: 90,
      },
      isLoading: false,
    });
    mockUseConfigQuery.mockReturnValue({
      data: {
        Server: { OpenBrowser: true },
        Monitoring: { Interval: 1_000_000_000, ProcessTreeInterval: 2_000_000_000, PortScanInterval: 3_000_000_000, GPUInterval: 2_000_000_000, HistoryDuration: 600_000_000_000, MaxProcesses: 2000 },
        Controller: { ConfirmKillSystem: true, ProtectedProcesses: [] },
        Anomaly: { IgnoreProcesses: [] },
        Notifications: { TrayBalloon: true, BalloonRateLimit: 30_000_000_000, BalloonMinSeverity: "critical" },
        UI: { Theme: "system", DefaultSort: "cpu", DefaultSortOrder: "desc", SparklinePoints: 60, ProcessTablePageSize: 100, RefreshRate: 1_000_000_000 },
      },
      isLoading: false,
    });
    mockUseAIConfigMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
    mockUseTelegramConfigMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
    mockUseConfigUpdateMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
    mockUseTelegramTestMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
    mockUseAITestMutation.mockReturnValue({ isPending: false, mutate: vi.fn() });
  });

  it("renders the missing settings controls again", () => {
    render(<SettingsPage />, { wrapper: TestWrapper });

    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByLabelText("AI provider")).toHaveValue("openai");
    expect(screen.getByLabelText("AI endpoint")).toHaveValue("https://api.openai.com/v1/chat/completions");
    expect(screen.getByLabelText("Telegram API base URL")).toHaveValue("https://api.telegram.org");
    expect(screen.getByLabelText("Telegram notification mode")).toHaveValue("high_value");
    expect(screen.getByLabelText("Telegram type Runaway CPU")).toBeChecked();
    expect(screen.getByRole("button", { name: "Save AI settings" })).toBeInTheDocument();
  });

  it("saves AI settings from the settings page", async () => {
    const user = userEvent.setup();
    const mutate = vi.fn();
    mockUseAIConfigMutation.mockReturnValue({ isPending: false, mutate });

    render(<SettingsPage />);

    await user.clear(screen.getByLabelText("AI model"));
    await user.type(screen.getByLabelText("AI model"), "gpt-5");
    await user.click(screen.getByRole("button", { name: "Save AI settings" }));

    expect(mutate).toHaveBeenCalled();
  });
});
