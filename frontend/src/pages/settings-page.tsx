import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Bot, Cable, Cog, Settings2 } from "lucide-react";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { PageSkeleton } from "../components/shared/page-skeleton";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import {
  useAIConfigMutation,
  useAIConfigQuery,
  useAITestMutation,
  useAIPresetsQuery,
  useConfigQuery,
  useConfigUpdateMutation,
  useTelegramConfigMutation,
  useTelegramConfigQuery,
  useTelegramTestMutation,
} from "../lib/api-client";
import type { AIConfig, TelegramConfig } from "../types/api";

const telegramAlertTypeOptions = [
  { value: "runaway_cpu", label: "Runaway CPU" },
  { value: "memory_leak", label: "Memory leak" },
  { value: "port_conflict", label: "Port conflict" },
  { value: "new_process", label: "Suspicious new process" },
  { value: "network_anomaly", label: "Network anomaly" },
  { value: "network_anomaly_system", label: "System-wide network anomaly" },
  { value: "rule:*", label: "Rule-triggered critical actions" },
];

function headersToText(headers: Record<string, string> | undefined) {
  if (!headers) {
    return "";
  }
  return Object.entries(headers)
    .map(([key, value]) => `${key}: ${value}`)
    .join("\n");
}

function textToHeaders(text: string) {
  const output: Record<string, string> = {};
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }
    const separatorIndex = line.indexOf(":");
    if (separatorIndex <= 0) {
      continue;
    }
    const key = line.slice(0, separatorIndex).trim();
    const value = line.slice(separatorIndex + 1).trim();
    if (key) {
      output[key] = value;
    }
  }
  return output;
}

function durationToMS(ns: number | undefined) {
  return Math.max(100, Math.round((ns ?? 0) / 1_000_000));
}

function durationToSec(ns: number | undefined) {
  return Math.max(1, Math.round((ns ?? 0) / 1_000_000_000));
}

export function SettingsPage() {
  const { data: aiConfig, isLoading: aiLoading } = useAIConfigQuery();
  const { data: aiPresets = [] } = useAIPresetsQuery();
  const { data: telegramConfig, isLoading: telegramLoading } = useTelegramConfigQuery();
  const { data: appConfig, isLoading: configLoading } = useConfigQuery();
  const aiMutation = useAIConfigMutation();
  const telegramMutation = useTelegramConfigMutation();
  const configMutation = useConfigUpdateMutation();
  const telegramTestMutation = useTelegramTestMutation();
  const aiTestMutation = useAITestMutation();
  const [selectedPreset, setSelectedPreset] = useState("");
  const [aiDirty, setAiDirty] = useState(false);
  const [telegramDirty, setTelegramDirty] = useState(false);
  const [runtimeDirty, setRuntimeDirty] = useState(false);
  const [aiForm, setAIForm] = useState<AIConfig>({
    enabled: false,
    provider: "anthropic",
    api_key: "",
    model: "",
    endpoint: "",
    extra_headers: {},
    language: "en",
    max_tokens: 1024,
    max_requests_per_minute: 5,
    include_process_tree: true,
    include_port_map: true,
  });
  const [aiHeadersText, setAIHeadersText] = useState("");
  const [telegramForm, setTelegramForm] = useState<TelegramConfig>({
    enabled: false,
    bot_token: "",
    allowed_chat_ids: [],
    api_base_url: "https://api.telegram.org",
    poll_timeout_sec: 25,
    notify_on_critical: true,
    notification_mode: "high_value",
    notification_types: ["runaway_cpu", "memory_leak", "port_conflict", "new_process", "network_anomaly", "network_anomaly_system", "rule:*"],
    require_confirm: true,
    confirm_ttl_sec: 90,
  });
  const [telegramChatIDsText, setTelegramChatIDsText] = useState("");
  const [runtimeForm, setRuntimeForm] = useState({
    openBrowser: true,
    intervalMS: 1000,
    processTreeIntervalMS: 2000,
    portScanIntervalMS: 3000,
    gpuIntervalMS: 2000,
    historyDurationSec: 600,
    maxProcesses: 2000,
    confirmKillSystem: true,
    trayBalloon: true,
    balloonRateLimitSec: 30,
    balloonMinSeverity: "critical",
    theme: "system",
    defaultSort: "cpu",
    defaultSortOrder: "desc",
    sparklinePoints: 60,
    processTablePageSize: 100,
    refreshRateMS: 1000,
  });

  useEffect(() => {
    if (!aiConfig) {
      return;
    }
    setAIForm({ ...aiConfig, api_key: "" });
    setAIHeadersText(headersToText(aiConfig.extra_headers));
  }, [aiConfig]);

  useEffect(() => {
    if (!telegramConfig) {
      return;
    }
    setTelegramForm({ ...telegramConfig, bot_token: "" });
    setTelegramChatIDsText((telegramConfig.allowed_chat_ids ?? []).join(", "));
  }, [telegramConfig]);

  useEffect(() => {
    if (!appConfig) {
      return;
    }
    setRuntimeForm({
      openBrowser: appConfig.Server.OpenBrowser,
      intervalMS: durationToMS(appConfig.Monitoring.Interval),
      processTreeIntervalMS: durationToMS(appConfig.Monitoring.ProcessTreeInterval),
      portScanIntervalMS: durationToMS(appConfig.Monitoring.PortScanInterval),
      gpuIntervalMS: durationToMS(appConfig.Monitoring.GPUInterval),
      historyDurationSec: durationToSec(appConfig.Monitoring.HistoryDuration),
      maxProcesses: appConfig.Monitoring.MaxProcesses,
      confirmKillSystem: appConfig.Controller.ConfirmKillSystem,
      trayBalloon: appConfig.Notifications.TrayBalloon,
      balloonRateLimitSec: durationToSec(appConfig.Notifications.BalloonRateLimit),
      balloonMinSeverity: appConfig.Notifications.BalloonMinSeverity,
      theme: appConfig.UI.Theme,
      defaultSort: appConfig.UI.DefaultSort,
      defaultSortOrder: appConfig.UI.DefaultSortOrder,
      sparklinePoints: appConfig.UI.SparklinePoints,
      processTablePageSize: appConfig.UI.ProcessTablePageSize,
      refreshRateMS: durationToMS(appConfig.UI.RefreshRate),
    });
  }, [appConfig]);

  const aiKeyState = useMemo(() => {
    if (!aiConfig?.api_key) {
      return "No key currently saved";
    }
    return `Current key: ${aiConfig.api_key} - leave blank to keep`;
  }, [aiConfig?.api_key]);

  const telegramTokenState = useMemo(() => {
    if (!telegramConfig?.bot_token) {
      return "No bot token currently saved";
    }
    return `Current token: ${telegramConfig.bot_token} - leave blank to keep`;
  }, [telegramConfig?.bot_token]);

  if (aiLoading || telegramLoading || configLoading) {
    return <PageSkeleton />;
  }

  if (!aiConfig || !telegramConfig || !appConfig) {
    return <EmptyState icon={Settings2} title="Settings are unavailable" description="WTM could not load one or more local configuration endpoints." />;
  }

  const controlClassName = "form-control w-full";
  const textareaClassName = "form-textarea";

  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description="Configure AI, Telegram, collector cadence, notifications, and local UI defaults. Everything here writes back to the local service."
        eyebrow="Configuration"
        icon={Settings2}
        meta={
          <>
            <Badge variant={aiForm.enabled ? "success" : "neutral"}>{aiForm.enabled ? "AI on" : "AI off"}</Badge>
            <Badge variant={telegramForm.enabled ? "info" : "neutral"}>{telegramForm.enabled ? "Telegram on" : "Telegram off"}</Badge>
            <Badge variant="info">{runtimeForm.theme} theme</Badge>
            {(aiDirty || telegramDirty || runtimeDirty) && (
              <Badge variant="warning">● unsaved</Badge>
            )}
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <MetricCard label="AI provider" value={aiConfig.provider || "anthropic"} badge={aiConfig.enabled ? "Enabled" : "Disabled"} badgeVariant={aiConfig.enabled ? "success" : "warning"} />
        <MetricCard label="Telegram" value={telegramConfig.enabled ? "Online" : "Off"} badge={telegramConfig.notify_on_critical ? "Critical alerts on" : "Quiet"} badgeVariant={telegramConfig.enabled ? "info" : "neutral"} />
        <MetricCard label="Dashboard runtime" value={`${runtimeForm.intervalMS} ms`} badge={runtimeForm.theme} badgeVariant="info" />
      </div>

      <Card className="space-y-4">
        <SectionTitle icon={Bot} title="AI provider settings" description="Choose the provider, model, limits, and context that power the local advisor." />
        <Subsection title="Routing and model">
          <div className="grid gap-3 lg:grid-cols-2">
            <Field label="Preset">
              <select
                aria-label="AI preset"
                className={controlClassName}
                value={selectedPreset}
                onChange={(event) => {
                  const presetID = event.target.value;
                  setSelectedPreset(presetID);
                  const preset = aiPresets.find((entry) => entry.id === presetID);
                  if (!preset) {
                    return;
                  }
                  setAIForm((current) => {
                    setAiDirty(true);
                    return {
                      ...current,
                      provider: preset.provider,
                      endpoint: preset.endpoint || current.endpoint,
                      model: preset.model || current.model,
                    };
                  });
                  setAIHeadersText(headersToText(preset.extra_headers));
                  setAiDirty(true);
                }}
              >
                <option value="">Choose a starter preset</option>
                {aiPresets.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {preset.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Provider">
              <select aria-label="AI provider" className={controlClassName} value={aiForm.provider} onChange={(event) => {
                  setAiDirty(true);
                  setAIForm((current) => ({ ...current, provider: event.target.value }));
                }}>
                <option value="anthropic">Anthropic</option>
                <option value="openai">OpenAI compatible</option>
              </select>
            </Field>
            <Field label="Model">
              <input aria-label="AI model" className={controlClassName} value={aiForm.model} onChange={(event) => {
                  setAiDirty(true);
                  setAIForm((current) => ({ ...current, model: event.target.value }));
                }} />
            </Field>
            <Field label="Endpoint">
              <input aria-label="AI endpoint" className={controlClassName} value={aiForm.endpoint} onChange={(event) => {
                  setAiDirty(true);
                  setAIForm((current) => ({ ...current, endpoint: event.target.value }));
                }} />
            </Field>
          </div>
        </Subsection>
        <Subsection title="Auth and limits">
          <div className="grid gap-3 lg:grid-cols-2">
            <Field label="API key">
              <div>
                <input
                  aria-label="AI API key"
                  type="password"
                  placeholder="Leave blank to keep current key"
                  className={controlClassName}
                  value={aiForm.api_key}
                  onChange={(event) => { setAiDirty(true); setAIForm((current) => ({ ...current, api_key: event.target.value })); }}
                />
                <div className="mt-2 text-xs text-secondary">{aiKeyState}</div>
              </div>
            </Field>
            <Field label="Language">
              <input aria-label="AI language" className={controlClassName} value={aiForm.language} onChange={(event) => { setAiDirty(true); setAIForm((current) => ({ ...current, language: event.target.value })); }} />
            </Field>
            <Field label="Max tokens">
              <input aria-label="AI max tokens" type="number" min={64} className={controlClassName} value={aiForm.max_tokens} onChange={(event) => { setAiDirty(true); setAIForm((current) => ({ ...current, max_tokens: Number(event.target.value) || 64 })); }} />
            </Field>
            <Field label="Requests per minute">
              <input aria-label="AI requests per minute" type="number" min={1} className={controlClassName} value={aiForm.max_requests_per_minute} onChange={(event) => { setAiDirty(true); setAIForm((current) => ({ ...current, max_requests_per_minute: Number(event.target.value) || 1 })); }} />
            </Field>
          </div>
        </Subsection>
        <Subsection title="Context">
          <Field label="Extra headers">
            <textarea aria-label="AI extra headers" className={textareaClassName} placeholder="HTTP-Referer: http://localhost" value={aiHeadersText} onChange={(event) => { setAiDirty(true); setAIHeadersText(event.target.value); }} />
          </Field>
          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            <Toggle label="Enable AI" checked={aiForm.enabled} onChange={(checked) => { setAiDirty(true); setAIForm((current) => ({ ...current, enabled: checked })); }} />
            <Toggle label="Include process tree" checked={aiForm.include_process_tree} onChange={(checked) => { setAiDirty(true); setAIForm((current) => ({ ...current, include_process_tree: checked })); }} />
            <Toggle label="Include port map" checked={aiForm.include_port_map} onChange={(checked) => { setAiDirty(true); setAIForm((current) => ({ ...current, include_port_map: checked })); }} />
          </div>
        </Subsection>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button" disabled={aiMutation.isPending} onClick={() => aiMutation.mutate({ ...aiForm, extra_headers: textToHeaders(aiHeadersText) }, { onSuccess: () => setAiDirty(false) })}>
            Save AI settings
          </Button>
          <Button
            type="button"
            variant="secondary"
            disabled={aiTestMutation.isPending || !aiForm.api_key || !aiForm.enabled}
            onClick={() => aiTestMutation.mutate()}
          >
            {aiTestMutation.isPending ? "Testing..." : aiTestMutation.isSuccess ? "Connection OK!" : "Test AI"}
          </Button>
          <span className="text-sm text-secondary">Chat stays on the AI page; provider setup lives here.</span>
        </div>
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="space-y-4">
          <SectionTitle icon={Cable} title="Telegram settings" description="Bot token, allowed chats, and confirmation behavior." />
          <Subsection title="Identity and delivery">
            <div className="grid gap-3">
              <Field label="Bot token">
                <div>
                  <input aria-label="Telegram bot token" type="password" placeholder="Leave blank to keep current token" className={controlClassName} value={telegramForm.bot_token} onChange={(event) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, bot_token: event.target.value })); }} />
                  <div className="mt-2 text-xs text-secondary">{telegramTokenState}</div>
                </div>
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Allowed chat IDs">
                  <input aria-label="Telegram allowed chat IDs" className={controlClassName} value={telegramChatIDsText} onChange={(event) => { setTelegramDirty(true); setTelegramChatIDsText(event.target.value); }} />
                </Field>
                <Field label="API base URL">
                  <input aria-label="Telegram API base URL" className={controlClassName} value={telegramForm.api_base_url} onChange={(event) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, api_base_url: event.target.value })); }} />
                </Field>
              </div>
            </div>
          </Subsection>
          <Subsection title="Notification policy">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Notification mode">
                <select aria-label="Telegram notification mode" className={controlClassName} value={telegramForm.notification_mode} onChange={(event) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, notification_mode: event.target.value })); }}>
                  <option value="high_value">Only high-value critical alerts</option>
                  <option value="all_critical">All critical alerts</option>
                </select>
              </Field>
              <Field label="Poll timeout (sec)">
                <input aria-label="Telegram poll timeout" type="number" min={5} className={controlClassName} value={telegramForm.poll_timeout_sec} onChange={(event) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, poll_timeout_sec: Number(event.target.value) || 5 })); }} />
              </Field>
              <Field label="Confirm TTL (sec)">
                <input aria-label="Telegram confirm TTL" type="number" min={15} className={controlClassName} value={telegramForm.confirm_ttl_sec} onChange={(event) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, confirm_ttl_sec: Number(event.target.value) || 15 })); }} />
              </Field>
            </div>
            <Field label="High-value alert types">
              <div className="grid gap-2 sm:grid-cols-2">
                {telegramAlertTypeOptions.map((option) => {
                  const checked = telegramForm.notification_types.includes(option.value);
                  return (
                    <label key={option.value} className="flex items-center gap-3 rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground">
                      <input
                        aria-label={`Telegram type ${option.label}`}
                        type="checkbox"
                        checked={checked}
                        onChange={(event) =>
                          { setTelegramDirty(true); setTelegramForm((current) => ({
                            ...current,
                            notification_types: event.target.checked
                              ? [...current.notification_types, option.value]
                              : current.notification_types.filter((item) => item !== option.value),
                          })); }
                        }
                      />
                      <span>{option.label}</span>
                    </label>
                  );
                })}
              </div>
            </Field>
            <div className="mt-3 grid gap-2 sm:grid-cols-3">
              <Toggle label="Enable Telegram" checked={telegramForm.enabled} onChange={(checked) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, enabled: checked })); }} />
              <Toggle label="Notify on critical" checked={telegramForm.notify_on_critical} onChange={(checked) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, notify_on_critical: checked })); }} />
              <Toggle label="Require confirm" checked={telegramForm.require_confirm} onChange={(checked) => { setTelegramDirty(true); setTelegramForm((current) => ({ ...current, require_confirm: checked })); }} />
            </div>
          </Subsection>
          <div className="soft-panel text-sm leading-6 text-secondary">
            <span className="font-semibold text-foreground">High-value mode</span> suppresses noisy `hung_process` and `spawn_storm` floods and keeps Telegram for genuinely actionable critical alerts.
          </div>
          <Button
            type="button"
            disabled={telegramMutation.isPending}
            onClick={() =>
              telegramMutation.mutate({
                ...telegramForm,
                allowed_chat_ids: telegramChatIDsText
                  .split(",")
                  .map((value) => value.trim())
                  .filter(Boolean)
                  .map((value) => Number(value))
                  .filter((value) => Number.isFinite(value)),
                notification_types: Array.from(new Set(telegramForm.notification_types)),
              }, { onSuccess: () => setTelegramDirty(false) })
            }
          >
            Save Telegram settings
          </Button>
          <Button
            type="button"
            variant="secondary"
            disabled={telegramTestMutation.isPending || !telegramConfig?.bot_token}
            onClick={() => telegramTestMutation.mutate()}
          >
            {telegramTestMutation.isPending ? "Testing..." : telegramTestMutation.isSuccess ? "Token valid!" : "Test bot"}
          </Button>
        </Card>

        <Card className="space-y-4">
          <SectionTitle icon={Cog} title="Runtime and dashboard" description="Browser launch, collector cadence, notifications, and UI defaults." />
          <Subsection title="Collector cadence">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Collector interval (ms)">
                <input aria-label="Collector interval" type="number" min={100} className={controlClassName} value={runtimeForm.intervalMS} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, intervalMS: Number(event.target.value) || 100 })); }} />
              </Field>
              <Field label="Process tree interval (ms)">
                <input aria-label="Process tree interval" type="number" min={100} className={controlClassName} value={runtimeForm.processTreeIntervalMS} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, processTreeIntervalMS: Number(event.target.value) || 100 })); }} />
              </Field>
              <Field label="Port scan interval (ms)">
                <input aria-label="Port scan interval" type="number" min={250} className={controlClassName} value={runtimeForm.portScanIntervalMS} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, portScanIntervalMS: Number(event.target.value) || 250 })); }} />
              </Field>
              <Field label="GPU interval (ms)">
                <input aria-label="GPU interval" type="number" min={250} className={controlClassName} value={runtimeForm.gpuIntervalMS} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, gpuIntervalMS: Number(event.target.value) || 250 })); }} />
              </Field>
              <Field label="History duration (sec)">
                <input aria-label="History duration" type="number" min={60} className={controlClassName} value={runtimeForm.historyDurationSec} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, historyDurationSec: Number(event.target.value) || 60 })); }} />
              </Field>
              <Field label="Max processes">
                <input aria-label="Max processes" type="number" min={100} className={controlClassName} value={runtimeForm.maxProcesses} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, maxProcesses: Number(event.target.value) || 100 })); }} />
              </Field>
            </div>
          </Subsection>
          <Subsection title="Dashboard defaults">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Refresh rate (ms)">
                <input aria-label="Refresh rate" type="number" min={100} className={controlClassName} value={runtimeForm.refreshRateMS} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, refreshRateMS: Number(event.target.value) || 100 })); }} />
              </Field>
              <Field label="Theme">
                <select aria-label="Default theme" className={controlClassName} value={runtimeForm.theme} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, theme: event.target.value })); }}>
                  <option value="system">System</option>
                  <option value="light">Light</option>
                  <option value="dark">Dark</option>
                </select>
              </Field>
              <Field label="Default process sort">
                <select aria-label="Default process sort" className={controlClassName} value={runtimeForm.defaultSort} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, defaultSort: event.target.value })); }}>
                  <option value="cpu">CPU</option>
                  <option value="memory">Memory</option>
                  <option value="name">Name</option>
                  <option value="pid">PID</option>
                  <option value="threads">Threads</option>
                </select>
              </Field>
              <Field label="Default sort order">
                <select aria-label="Default sort order" className={controlClassName} value={runtimeForm.defaultSortOrder} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, defaultSortOrder: event.target.value })); }}>
                  <option value="desc">Descending</option>
                  <option value="asc">Ascending</option>
                </select>
              </Field>
              <Field label="Sparkline points">
                <input aria-label="Sparkline points" type="number" min={10} className={controlClassName} value={runtimeForm.sparklinePoints} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, sparklinePoints: Number(event.target.value) || 10 })); }} />
              </Field>
              <Field label="Process table page size">
                <input aria-label="Process table page size" type="number" min={10} className={controlClassName} value={runtimeForm.processTablePageSize} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, processTablePageSize: Number(event.target.value) || 10 })); }} />
              </Field>
              <Field label="Balloon rate limit (sec)">
                <input aria-label="Balloon rate limit" type="number" min={1} className={controlClassName} value={runtimeForm.balloonRateLimitSec} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, balloonRateLimitSec: Number(event.target.value) || 1 })); }} />
              </Field>
              <Field label="Balloon min severity">
                <select aria-label="Balloon min severity" className={controlClassName} value={runtimeForm.balloonMinSeverity} onChange={(event) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, balloonMinSeverity: event.target.value })); }}>
                  <option value="info">Info</option>
                  <option value="warning">Warning</option>
                  <option value="critical">Critical</option>
                </select>
              </Field>
            </div>
          </Subsection>
          <div className="grid gap-2 sm:grid-cols-2">
            <Toggle label="Open browser on start" checked={runtimeForm.openBrowser} onChange={(checked) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, openBrowser: checked })); }} />
            <Toggle label="Confirm kill system" checked={runtimeForm.confirmKillSystem} onChange={(checked) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, confirmKillSystem: checked })); }} />
            <Toggle label="Tray balloon alerts" checked={runtimeForm.trayBalloon} onChange={(checked) => { setRuntimeDirty(true); setRuntimeForm((current) => ({ ...current, trayBalloon: checked })); }} />
          </div>
          <Button
            type="button"
            disabled={configMutation.isPending}
            onClick={() =>
              configMutation.mutate({
                server: { open_browser: runtimeForm.openBrowser },
                monitoring: {
                  interval_ms: runtimeForm.intervalMS,
                  process_tree_interval_ms: runtimeForm.processTreeIntervalMS,
                  port_scan_interval_ms: runtimeForm.portScanIntervalMS,
                  gpu_interval_ms: runtimeForm.gpuIntervalMS,
                  history_duration_sec: runtimeForm.historyDurationSec,
                  max_processes: runtimeForm.maxProcesses,
                },
                controller: { confirm_kill_system: runtimeForm.confirmKillSystem },
                notifications: {
                  tray_balloon: runtimeForm.trayBalloon,
                  balloon_rate_limit_sec: runtimeForm.balloonRateLimitSec,
                  balloon_min_severity: runtimeForm.balloonMinSeverity,
                },
                ui: {
                  theme: runtimeForm.theme,
                  default_sort: runtimeForm.defaultSort,
                  default_sort_order: runtimeForm.defaultSortOrder,
                  sparkline_points: runtimeForm.sparklinePoints,
                  process_table_page_size: runtimeForm.processTablePageSize,
                  refresh_rate_ms: runtimeForm.refreshRateMS,
                },
              }, { onSuccess: () => setRuntimeDirty(false) })
            }
          >
            Save runtime settings
          </Button>
        </Card>
      </div>
    </div>
  );
}

interface MetricCardProps {
  label: string;
  value: string;
  badge: string;
  badgeVariant: "neutral" | "info" | "success" | "warning";
}

function MetricCard({ label, value, badge, badgeVariant }: MetricCardProps) {
  return (
    <Card>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="metric-label">{label}</div>
          <div className="mt-2 truncate text-xl font-semibold tracking-tight text-foreground sm:text-2xl">{value}</div>
        </div>
        <Badge variant={badgeVariant}>{badge}</Badge>
      </div>
    </Card>
  );
}

interface SectionTitleProps {
  icon: typeof Bot;
  title: string;
  description: string;
}

function SectionTitle({ icon: Icon, title, description }: SectionTitleProps) {
  return (
    <div className="flex items-start gap-3">
      <div className="rounded-md border border-border bg-background px-2.5 py-2 text-accent">
        <Icon className="h-4 w-4" />
      </div>
      <div>
        <div className="text-base font-semibold tracking-tight text-foreground sm:text-lg">{title}</div>
        <div className="text-sm leading-6 text-secondary">{description}</div>
      </div>
    </div>
  );
}

interface FieldProps {
  label: string;
  children: ReactNode;
}

function Field({ label, children }: FieldProps) {
  return (
    <label className="block space-y-2">
      <span className="text-sm font-medium text-foreground">{label}</span>
      {children}
    </label>
  );
}

interface SubsectionProps {
  title: string;
  children: ReactNode;
}

function Subsection({ title, children }: SubsectionProps) {
  return (
    <div className="soft-panel space-y-3">
      <div className="text-sm font-semibold text-foreground">{title}</div>
      {children}
    </div>
  );
}

interface ToggleProps {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}

function Toggle({ label, checked, onChange }: ToggleProps) {
  return (
    <label className="flex min-h-10 items-center justify-between rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground">
      <span>{label}</span>
      <input aria-label={label} type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
    </label>
  );
}
