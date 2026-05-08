import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import type { AIConfig, AIResult, AISuggestion, AIStatus, AIPreset, AlertItem, AppConfig, InfoResponse, PortBinding, Rule, RulesResponse, SystemSnapshot, TelegramConfig } from "../types/api";
import { queryClient } from "./query-client";

const API_BASE = "/api/v1";
const csrfToken = document.querySelector('meta[name="wtm-csrf-token"]')?.getAttribute("content") ?? "";

interface ApiErrorShape {
  code?: string;
  message?: string;
  details?: string;
}

interface ApiErrorEnvelope {
  error?: ApiErrorShape;
}

class ApiError extends Error {
  code?: string;
  details?: string;

  constructor(message: string, code?: string, details?: string) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.details = details;
  }
}

export function extractApiErrorShape(payload: unknown): ApiErrorShape {
  if (!payload || typeof payload !== "object") {
    return {};
  }
  const direct = payload as ApiErrorShape;
  if (direct.message || direct.code || direct.details) {
    return direct;
  }
  const nested = (payload as ApiErrorEnvelope).error;
  if (nested && typeof nested === "object") {
    return nested;
  }
  return {};
}

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const method = (init?.method ?? "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD" && method !== "OPTIONS" && csrfToken) {
    headers.set("X-WTM-CSRF", csrfToken);
  }
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: "same-origin",
    headers,
  });
  if (!response.ok) {
    const contentType = response.headers.get("content-type") ?? "";
    let message = `Request failed: ${response.status}`;
    let code: string | undefined;
    let details: string | undefined;

    if (contentType.includes("application/json")) {
      const payload = extractApiErrorShape(await response.json());
      message = payload.message ?? message;
      code = payload.code;
      details = payload.details;
    } else {
      const text = await response.text();
      if (text.trim()) {
        message = text.trim();
      }
    }

    throw new ApiError(message, code, details);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export async function apiGet<T>(path: string, options?: { params?: Record<string, string> }): Promise<T> {
  let url = path;
  if (options?.params) {
    const searchParams = new URLSearchParams(options.params);
    url += `?${searchParams.toString()}`;
  }
  return apiRequest<T>(url);
}

async function apiPost<TBody extends object, TResponse>(path: string, body: TBody): Promise<TResponse> {
  return apiRequest<TResponse>(path, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
}

async function apiPut<TBody extends object, TResponse>(path: string, body: TBody): Promise<TResponse> {
  return apiRequest<TResponse>(path, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
}

async function apiPostNoBody<TResponse>(path: string): Promise<TResponse> {
  return apiRequest<TResponse>(path, { method: "POST" });
}

export function useSystemSnapshotQuery() {
  return useQuery({
    queryKey: ["system"],
    queryFn: () => apiGet<SystemSnapshot>("/system"),
    staleTime: 1_000,
    refetchInterval: 1_000,
  });
}

export function useAlertsQuery() {
  return useQuery({
    queryKey: ["alerts"],
    queryFn: async () => {
      const [active, history] = await Promise.all([
        apiGet<AlertItem[]>("/alerts"),
        apiGet<AlertItem[]>("/alerts/history"),
      ]);
      return { active, history };
    },
    refetchInterval: 5_000,
  });
}

export function useRulesQuery() {
  return useQuery({
    queryKey: ["rules"],
    queryFn: () => apiGet<RulesResponse>("/rules"),
  });
}

export function useRulesUpdateMutation() {
  return useMutation({
    mutationFn: (rules: Rule[]) => apiPost<{ rules: Rule[] }, { ok: boolean; rules: Rule[] }>("/rules", { rules }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["rules"] });
      toast.success("Rules updated.");
    },
  });
}

export function useAIStatusQuery() {
  return useQuery({
    queryKey: ["ai-status"],
    queryFn: () => apiGet<AIStatus>("/ai/status"),
    refetchInterval: 10_000,
  });
}

export function useConfigQuery() {
  return useQuery({
    queryKey: ["config"],
    queryFn: () => apiGet<AppConfig>("/config"),
    staleTime: 15_000,
  });
}

export function useInfoQuery() {
  return useQuery({
    queryKey: ["info"],
    queryFn: () => apiGet<InfoResponse>("/info"),
    staleTime: 30_000,
  });
}

export function useAIConfigQuery() {
  return useQuery({
    queryKey: ["ai-config"],
    queryFn: () => apiGet<AIConfig>("/ai/config"),
    staleTime: 15_000,
  });
}

export function useAIPresetsQuery() {
  return useQuery({
    queryKey: ["ai-presets"],
    queryFn: () => apiGet<AIPreset[]>("/ai/presets"),
    staleTime: 60_000,
  });
}

export function useTelegramConfigQuery() {
  return useQuery({
    queryKey: ["telegram-config"],
    queryFn: () => apiGet<TelegramConfig>("/telegram/config"),
    staleTime: 15_000,
  });
}

export function useProcessConnectionsQuery(pid: number | null) {
  return useQuery({
    queryKey: ["process-connections", pid],
    queryFn: () => apiGet<PortBinding[]>(`/processes/${pid}/connections`),
    enabled: pid !== null,
    staleTime: 5_000,
    refetchInterval: 5_000,
  });
}

export function useAIChatMutation() {
  return useMutation({
    mutationFn: (message: string) => apiPost<{ message: string }, AIResult>("/ai/chat", { message }),
    onSuccess: () => {
      toast.success("AI response received.");
    },
  });
}

export function useAIAnalyzeMutation() {
  return useMutation({
    mutationFn: (prompt: string) => apiPost<{ prompt: string }, AIResult>("/ai/analyze", { prompt }),
    onSuccess: () => {
      toast.success("AI analysis received.");
    },
  });
}

export function useAIExecuteMutation() {
  return useMutation({
    mutationFn: (suggestion: AISuggestion) => apiPost("/ai/execute", { ...suggestion, confirm: true } as AISuggestion & { confirm: boolean }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["system"] }),
        queryClient.invalidateQueries({ queryKey: ["alerts"] }),
        queryClient.invalidateQueries({ queryKey: ["rules"] }),
        queryClient.invalidateQueries({ queryKey: ["config"] }),
      ]);
      toast.success("AI action approved.");
    },
  });
}

export function useAIConfigMutation() {
  return useMutation({
    mutationFn: (payload: AIConfig) => apiPost<AIConfig, { ok: boolean; config: AIConfig }>("/ai/config", payload),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["ai-config"] }),
        queryClient.invalidateQueries({ queryKey: ["ai-status"] }),
      ]);
      toast.success("AI settings saved.");
    },
  });
}

export function useTelegramConfigMutation() {
  return useMutation({
    mutationFn: (payload: TelegramConfig) =>
      apiPost<TelegramConfig, { ok: boolean; config: TelegramConfig }>("/telegram/config", payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["telegram-config"] });
      toast.success("Telegram settings saved.");
    },
  });
}

export function useTelegramTestMutation() {
  return useMutation({
    mutationFn: () => apiPostNoBody<{ ok: boolean; message?: string }>("/telegram/test"),
  });
}

export function useAITestMutation() {
  return useMutation({
    mutationFn: () => apiPostNoBody<{ ok: boolean; response?: string }>("/ai/test"),
  });
}

interface ConfigUpdatePayload {
  server?: {
    open_browser?: boolean;
  };
  monitoring?: {
    interval_ms?: number;
    process_tree_interval_ms?: number;
    port_scan_interval_ms?: number;
    gpu_interval_ms?: number;
    history_duration_sec?: number;
    max_processes?: number;
  };
  controller?: {
    confirm_kill_system?: boolean;
  };
  notifications?: {
    tray_balloon?: boolean;
    balloon_rate_limit_sec?: number;
    balloon_min_severity?: string;
  };
  ui?: {
    theme?: string;
    default_sort?: string;
    default_sort_order?: string;
    sparkline_points?: number;
    process_table_page_size?: number;
    refresh_rate_ms?: number;
  };
}

export function useConfigUpdateMutation() {
  return useMutation({
    mutationFn: (payload: ConfigUpdatePayload) => apiPut<ConfigUpdatePayload, { ok: boolean; config: AppConfig }>("/config", payload),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["config"] }),
        queryClient.invalidateQueries({ queryKey: ["info"] }),
      ]);
      toast.success("Runtime settings saved.");
    },
  });
}

interface ProcessActionMutationOptions {
  successMessage?: string | false;
}

export function useProcessActionMutation(options?: ProcessActionMutationOptions) {
  return useMutation({
    mutationFn: ({ pid, action }: { pid: number; action: "kill" | "suspend" | "resume" }) =>
      apiPostNoBody<{ ok: boolean }>(`/processes/${pid}/${action}?confirm=true`),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["system"] }),
        queryClient.invalidateQueries({ queryKey: ["alerts"] }),
      ]);
      if (options?.successMessage !== false) {
        toast.success(options?.successMessage ?? "Process action completed.");
      }
    },
  });
}

export function useAlertActionMutation() {
  return useMutation({
    mutationFn: ({ type, pid, action }: { type: string; pid?: number; action: "dismiss" | "snooze" }) => {
      const pidPath = pid ? `/${pid}` : "";
      const suffix = action === "snooze" ? "?duration=30m" : "";
      return apiPostNoBody<{ ok: boolean }>(`/alerts/${encodeURIComponent(type)}${pidPath}/${action}${suffix}`);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["alerts"] });
      toast.success("Alert state updated.");
    },
  });
}
