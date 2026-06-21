import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Capture react-query configs so we can drive queryFn / mutationFn / onSuccess
// directly without needing a full QueryClient render tree.
const capturedQueryConfigs: Array<Record<string, unknown>> = [];
const capturedMutationConfigs: Array<Record<string, unknown>> = [];

vi.mock("@tanstack/react-query", () => ({
  useQuery: (config: Record<string, unknown>) => {
    capturedQueryConfigs.push(config);
    return { __queryConfig: config };
  },
  useMutation: (config: Record<string, unknown>) => {
    capturedMutationConfigs.push(config);
    return { __mutationConfig: config };
  },
}));

const toastMock = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
  loading: vi.fn(),
  promise: vi.fn(),
  dismiss: vi.fn(),
  custom: vi.fn(),
  message: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: toastMock,
}));

const queryClientMock = vi.hoisted(() => ({
  invalidateQueries: vi.fn(async () => undefined),
}));

vi.mock("./query-client", () => ({
  queryClient: queryClientMock,
}));

// Ensure the CSRF meta tag is present BEFORE api-client.ts is imported,
// because api-client.ts captures `csrfToken` as a module-level constant.
vi.hoisted(() => {
  document.head.innerHTML = "";
  const meta = document.createElement("meta");
  meta.setAttribute("name", "wtm-csrf-token");
  meta.setAttribute("content", "test-csrf-token");
  document.head.appendChild(meta);
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type CapturedQuery = {
  queryKey: unknown;
  queryFn: (...args: unknown[]) => Promise<unknown>;
  enabled?: unknown;
  staleTime?: unknown;
  refetchInterval?: unknown;
};

type CapturedMutation = {
  mutationFn: (...args: unknown[]) => Promise<unknown>;
  onSuccess?: (...args: unknown[]) => unknown;
};

interface MockResponseInit {
  ok?: boolean;
  status?: number;
  contentType?: string | null;
  json?: unknown;
  text?: string;
}

function jsonResponse(body: unknown, init: MockResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "content-type": init.contentType ?? "application/json" },
  });
}

function emptyResponse(init: MockResponseInit = {}): Response {
  return new Response(null, {
    status: init.status ?? 204,
    headers: init.contentType ? { "content-type": init.contentType } : undefined,
  });
}

function errorJsonResponse(body: unknown, status = 500): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function errorTextResponse(body: string, status = 500): Response {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain" },
  });
}

function lastQuery(): CapturedQuery {
  const cfg = capturedQueryConfigs[capturedQueryConfigs.length - 1];
  if (!cfg) throw new Error("no query was captured");
  return cfg as unknown as CapturedQuery;
}

function lastMutation(): CapturedMutation {
  const cfg = capturedMutationConfigs[capturedMutationConfigs.length - 1];
  if (!cfg) throw new Error("no mutation was captured");
  return cfg as unknown as CapturedMutation;
}

function setCsrfToken(value: string | null) {
  document.head.innerHTML = "";
  if (value !== null) {
    const meta = document.createElement("meta");
    meta.setAttribute("name", "wtm-csrf-token");
    meta.setAttribute("content", value);
    document.head.appendChild(meta);
  }
}

beforeEach(() => {
  capturedQueryConfigs.length = 0;
  capturedMutationConfigs.length = 0;
  queryClientMock.invalidateQueries.mockClear();
  queryClientMock.invalidateQueries.mockResolvedValue(undefined);
  toastMock.success.mockClear();
  toastMock.error.mockClear();
  toastMock.info.mockClear();
  toastMock.warning.mockClear();
  toastMock.loading.mockClear();
  toastMock.promise.mockClear();
  toastMock.dismiss.mockClear();
  toastMock.custom.mockClear();
  toastMock.message.mockClear();
  vi.restoreAllMocks();
  setCsrfToken("test-csrf-token");
});

afterEach(() => {
  setCsrfToken(null);
});

// Pull the module under test AFTER mocks are set up.
import * as api from "./api-client";

// ---------------------------------------------------------------------------
// extractApiErrorShape — pure function coverage
// ---------------------------------------------------------------------------

describe("extractApiErrorShape", () => {
  it("supports the backend error envelope", () => {
    expect(api.extractApiErrorShape({ error: { code: "invalid_param", message: "pid must be uint32" } })).toEqual({
      code: "invalid_param",
      message: "pid must be uint32",
    });
  });

  it("keeps direct error payloads working", () => {
    expect(api.extractApiErrorShape({ code: "bad_request", message: "broken", details: "extra" })).toEqual({
      code: "bad_request",
      message: "broken",
      details: "extra",
    });
  });

  it("returns an empty shape for null input", () => {
    expect(api.extractApiErrorShape(null)).toEqual({});
  });

  it("returns an empty shape for undefined input", () => {
    expect(api.extractApiErrorShape(undefined)).toEqual({});
  });

  it("returns an empty shape for primitive input", () => {
    expect(api.extractApiErrorShape("nope")).toEqual({});
    expect(api.extractApiErrorShape(42)).toEqual({});
    expect(api.extractApiErrorShape(true)).toEqual({});
  });

  it("returns an empty shape for an empty object", () => {
    expect(api.extractApiErrorShape({})).toEqual({});
  });

  it("returns an empty shape when envelope has no error field", () => {
    expect(api.extractApiErrorShape({ other: "thing" })).toEqual({});
  });

  it("returns an empty shape when nested error is not an object", () => {
    expect(api.extractApiErrorShape({ error: "not-an-object" })).toEqual({});
  });

  it("returns the nested error object when present", () => {
    expect(api.extractApiErrorShape({ error: { code: "x", message: "y" } })).toEqual({
      code: "x",
      message: "y",
    });
  });

  it("treats direct message alone as a direct error", () => {
    expect(api.extractApiErrorShape({ message: "boom" })).toEqual({ message: "boom" });
  });

  it("treats direct code alone as a direct error", () => {
    expect(api.extractApiErrorShape({ code: "x" })).toEqual({ code: "x" });
  });

  it("treats direct details alone as a direct error", () => {
    expect(api.extractApiErrorShape({ details: "x" })).toEqual({ details: "x" });
  });
});

// ---------------------------------------------------------------------------
// apiRequest — exercised indirectly via apiGet / queryFn / mutationFn
// ---------------------------------------------------------------------------

describe("apiRequest — success paths", () => {
  it("performs GET requests with same-origin credentials", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ hello: "world" }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await api.apiGet<{ hello: string }>("/foo");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/foo");
    expect(init.credentials).toBe("same-origin");
    // GET — no X-WTM-CSRF header
    const headers = new Headers(init.headers);
    expect(headers.get("X-WTM-CSRF")).toBeNull();
    expect(result).toEqual({ hello: "world" });
  });

  it("appends URL search params when provided", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await api.apiGet<{ ok: boolean }>("/things", { params: { a: "1", b: "two words" } });

    const url = fetchMock.mock.calls[0]![0];
    expect(url).toBe("/api/v1/things?a=1&b=two+words");
  });

  it("returns undefined for 204 responses", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => emptyResponse());
    vi.stubGlobal("fetch", fetchMock);

    const result = await api.apiGet<undefined>("/empty");
    expect(result).toBeUndefined();
  });
});

describe("apiRequest — CSRF token behavior", () => {
  it("attaches the CSRF header on POST", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useTelegramTestMutation();
    const mutationFn = lastMutation().mutationFn;
    await mutationFn();

    const init = fetchMock.mock.calls[0]![1];
    const headers = new Headers(init.headers);
    expect(init.method).toBe("POST");
    expect(headers.get("X-WTM-CSRF")).toBe("test-csrf-token");
  });

  it("attaches the CSRF header on PUT", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useConfigUpdateMutation();
    const mutationFn = lastMutation().mutationFn;
    await mutationFn({ ui: { theme: "dark" } });

    const init = fetchMock.mock.calls[0]![1];
    const headers = new Headers(init.headers);
    expect(init.method).toBe("PUT");
    expect(headers.get("X-WTM-CSRF")).toBe("test-csrf-token");
  });

  it("omits the CSRF header on GET requests", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);

    await api.apiGet("/whatever");

    const init = fetchMock.mock.calls[0]![1];
    const headers = new Headers(init.headers);
    expect(headers.get("X-WTM-CSRF")).toBeNull();
  });

  it("does not add CSRF header when token is missing", async () => {
    vi.resetModules();
    setCsrfToken(null);
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const fresh = await import("./api-client");
    fresh.useTelegramTestMutation();
    const mutationFn = lastMutation().mutationFn;
    await mutationFn();

    const init = fetchMock.mock.calls[0]![1];
    const headers = new Headers(init.headers);
    expect(headers.get("X-WTM-CSRF")).toBeNull();

    setCsrfToken("test-csrf-token");
  });

  it("does not add CSRF header for empty token string", async () => {
    vi.resetModules();
    setCsrfToken("");
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    const fresh = await import("./api-client");
    fresh.useTelegramTestMutation();
    const mutationFn = lastMutation().mutationFn;
    await mutationFn();

    const init = fetchMock.mock.calls[0]![1];
    const headers = new Headers(init.headers);
    expect(headers.get("X-WTM-CSRF")).toBeNull();

    setCsrfToken("test-csrf-token");
  });
});

describe("apiRequest — error paths", () => {
  it("throws ApiError with backend envelope fields", async () => {
    const fetchMock = vi.fn(
      async (_input: string, _init: RequestInit) => errorJsonResponse({ error: { code: "bad", message: "Bad thing", details: "more" } }, 400),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      code: "bad",
      message: "Bad thing",
      details: "more",
    });
  });

  it("falls back to default message when JSON envelope omits message", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorJsonResponse({ error: { code: "x" } }, 418));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      code: "x",
      message: "Request failed: 418",
    });
  });

  it("falls back to default message when JSON envelope is empty", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorJsonResponse({}, 502));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 502",
    });
  });

  it("treats a JSON envelope without an error key as an empty shape", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorJsonResponse({ unrelated: true }, 500));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 500",
    });
  });

  it("uses response text as message when content-type is not JSON and text is non-empty", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorTextResponse("oh no", 500));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "oh no",
    });
  });

  it("falls back to default message when text body is whitespace only", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorTextResponse("   ", 500));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 500",
    });
  });

  it("falls back to default message when text body is empty", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorTextResponse("", 500));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 500",
    });
  });

  it("uses trimmed text as message", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => errorTextResponse("  trimmed body  ", 500));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "trimmed body",
    });
  });

  it("handles a missing content-type header by treating as text (null body, no content-type derived)", async () => {
    // null body forces content-type header to be null, exercising the `?? ""` fallback
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => new Response(null, { status: 500, statusText: "Server Error" }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 500",
    });
  });

  it("treats non-JSON content-types as text", async () => {
    const fetchMock = vi.fn(
      async (_input: string, _init: RequestInit) =>
        new Response("html error page", {
          status: 500,
          headers: { "content-type": "text/html" },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "html error page",
    });
  });

  it("falls back to default message when non-JSON body is empty", async () => {
    const fetchMock = vi.fn(
      async (_input: string, _init: RequestInit) =>
        new Response("", {
          status: 500,
          headers: { "content-type": "text/html" },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.apiGet("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Request failed: 500",
    });
  });
});

// ---------------------------------------------------------------------------
// useSystemSnapshotQuery
// ---------------------------------------------------------------------------

describe("useSystemSnapshotQuery", () => {
  it("queries the /system endpoint with the expected configuration", () => {
    api.useSystemSnapshotQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["system"]);
    expect(cfg.staleTime).toBe(1_000);
    expect(cfg.refetchInterval).toBe(1_000);
    expect(typeof cfg.queryFn).toBe("function");
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ timestamp: "now" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useSystemSnapshotQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/system");
    expect(result).toEqual({ timestamp: "now" });
  });
});

// ---------------------------------------------------------------------------
// useAlertsQuery
// ---------------------------------------------------------------------------

describe("useAlertsQuery", () => {
  it("queries active and history alerts in parallel", () => {
    api.useAlertsQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["alerts"]);
    expect(cfg.refetchInterval).toBe(5_000);
  });

  it("queryFn returns active + history merged", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(async () => jsonResponse([{ type: "active1" }]))
      .mockImplementationOnce(async () => jsonResponse([{ type: "history1" }, { type: "history2" }]));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertsQuery();
    const result = await lastQuery().queryFn();

    const urls = fetchMock.mock.calls.map((c) => c[0]);
    expect(urls).toEqual(["/api/v1/alerts", "/api/v1/alerts/history"]);
    expect(result).toEqual({
      active: [{ type: "active1" }],
      history: [{ type: "history1" }, { type: "history2" }],
    });
  });
});

// ---------------------------------------------------------------------------
// useRulesQuery
// ---------------------------------------------------------------------------

describe("useRulesQuery", () => {
  it("queries the /rules endpoint", () => {
    api.useRulesQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["rules"]);
    expect(typeof cfg.queryFn).toBe("function");
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ rules: [] }));
    vi.stubGlobal("fetch", fetchMock);

    api.useRulesQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/rules");
    expect(result).toEqual({ rules: [] });
  });
});

// ---------------------------------------------------------------------------
// useRulesUpdateMutation
// ---------------------------------------------------------------------------

describe("useRulesUpdateMutation", () => {
  it("POSTs rules and invalidates the rules query", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, rules: [] }));
    vi.stubGlobal("fetch", fetchMock);

    api.useRulesUpdateMutation();
    const { mutationFn, onSuccess } = lastMutation();

    const rules = [
      {
        name: "r1",
        enabled: true,
        match: "x",
        metric: "cpu",
        op: ">",
        threshold: 90,
        for_seconds: 5,
        action: "alert",
        cooldown_seconds: 60,
      },
    ];
    await mutationFn(rules);

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/rules");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ rules }));
    expect(new Headers(init.headers).get("Content-Type")).toBe("application/json");

    if (onSuccess) {
      await Promise.resolve(onSuccess({ ok: true, rules }, rules, undefined));
    }
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["rules"] });
    expect(toastMock.success).toHaveBeenCalledWith("Rules updated.");
  });
});

// ---------------------------------------------------------------------------
// useAIStatusQuery
// ---------------------------------------------------------------------------

describe("useAIStatusQuery", () => {
  it("queries the /ai/status endpoint", () => {
    api.useAIStatusQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["ai-status"]);
    expect(cfg.refetchInterval).toBe(10_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ enabled: true, provider: "openai" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIStatusQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/ai/status");
    expect(result).toEqual({ enabled: true, provider: "openai" });
  });
});

// ---------------------------------------------------------------------------
// useConfigQuery
// ---------------------------------------------------------------------------

describe("useConfigQuery", () => {
  it("queries the /config endpoint", () => {
    api.useConfigQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["config"]);
    expect(cfg.staleTime).toBe(15_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ Server: { OpenBrowser: true } }));
    vi.stubGlobal("fetch", fetchMock);

    api.useConfigQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/config");
    expect(result).toEqual({ Server: { OpenBrowser: true } });
  });
});

// ---------------------------------------------------------------------------
// useInfoQuery
// ---------------------------------------------------------------------------

describe("useInfoQuery", () => {
  it("queries the /info endpoint", () => {
    api.useInfoQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["info"]);
    expect(cfg.staleTime).toBe(30_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ version: "0.4.3" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useInfoQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/info");
    expect(result).toEqual({ version: "0.4.3" });
  });
});

// ---------------------------------------------------------------------------
// useAIConfigQuery
// ---------------------------------------------------------------------------

describe("useAIConfigQuery", () => {
  it("queries the /ai/config endpoint", () => {
    api.useAIConfigQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["ai-config"]);
    expect(cfg.staleTime).toBe(15_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ provider: "anthropic" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIConfigQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/ai/config");
    expect(result).toEqual({ provider: "anthropic" });
  });
});

// ---------------------------------------------------------------------------
// useAIPresetsQuery
// ---------------------------------------------------------------------------

describe("useAIPresetsQuery", () => {
  it("queries the /ai/presets endpoint", () => {
    api.useAIPresetsQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["ai-presets"]);
    expect(cfg.staleTime).toBe(60_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse([{ id: "x" }]));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIPresetsQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/ai/presets");
    expect(result).toEqual([{ id: "x" }]);
  });
});

// ---------------------------------------------------------------------------
// useTelegramConfigQuery
// ---------------------------------------------------------------------------

describe("useTelegramConfigQuery", () => {
  it("queries the /telegram/config endpoint", () => {
    api.useTelegramConfigQuery();
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["telegram-config"]);
    expect(cfg.staleTime).toBe(15_000);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ enabled: false }));
    vi.stubGlobal("fetch", fetchMock);

    api.useTelegramConfigQuery();
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/telegram/config");
    expect(result).toEqual({ enabled: false });
  });
});

// ---------------------------------------------------------------------------
// useProcessConnectionsQuery
// ---------------------------------------------------------------------------

describe("useProcessConnectionsQuery", () => {
  it("queries /processes/<pid>/connections when pid is provided", () => {
    api.useProcessConnectionsQuery(123);
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["process-connections", 123]);
    expect(cfg.enabled).toBe(true);
    expect(cfg.staleTime).toBe(5_000);
    expect(cfg.refetchInterval).toBe(5_000);
  });

  it("disables the query when pid is null", () => {
    api.useProcessConnectionsQuery(null);
    const cfg = lastQuery();

    expect(cfg.queryKey).toEqual(["process-connections", null]);
    expect(cfg.enabled).toBe(false);
  });

  it("invokes the queryFn and returns parsed JSON", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse([{ local_port: 80 }]));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessConnectionsQuery(42);
    const result = await lastQuery().queryFn();

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/processes/42/connections");
    expect(result).toEqual([{ local_port: 80 }]);
  });
});

// ---------------------------------------------------------------------------
// useAIChatMutation
// ---------------------------------------------------------------------------

describe("useAIChatMutation", () => {
  it("POSTs the message and shows a success toast", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ answer: "ok" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIChatMutation();
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn("hello");

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/ai/chat");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ message: "hello" }));

    if (onSuccess) await Promise.resolve(onSuccess({ answer: "ok" }, "hello", undefined));
    expect(toastMock.success).toHaveBeenCalledWith("AI response received.");
  });
});

// ---------------------------------------------------------------------------
// useAIAnalyzeMutation
// ---------------------------------------------------------------------------

describe("useAIAnalyzeMutation", () => {
  it("POSTs the prompt and shows a success toast", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ answer: "analysis" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIAnalyzeMutation();
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn("analyze this");

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/ai/analyze");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ prompt: "analyze this" }));

    if (onSuccess) await Promise.resolve(onSuccess({ answer: "analysis" }, "analyze this", undefined));
    expect(toastMock.success).toHaveBeenCalledWith("AI analysis received.");
  });
});

// ---------------------------------------------------------------------------
// useAIExecuteMutation
// ---------------------------------------------------------------------------

describe("useAIExecuteMutation", () => {
  it("POSTs the suggestion with confirm=true and invalidates dependent queries", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIExecuteMutation();
    const { mutationFn, onSuccess } = lastMutation();

    const suggestion = {
      id: "s1",
      type: "kill",
      pid: 7,
      name: "thing",
      reason: "r",
      rule: undefined as unknown,
    };
    await mutationFn(suggestion);

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/ai/execute");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ ...suggestion, confirm: true }));

    if (onSuccess) await Promise.resolve(onSuccess({ ok: true }, suggestion, undefined));
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["system"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["alerts"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["rules"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["config"] });
    expect(toastMock.success).toHaveBeenCalledWith("AI action approved.");
  });
});

// ---------------------------------------------------------------------------
// useAIConfigMutation
// ---------------------------------------------------------------------------

describe("useAIConfigMutation", () => {
  it("POSTs the config and invalidates ai-config + ai-status", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, config: {} }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAIConfigMutation();
    const { mutationFn, onSuccess } = lastMutation();

    const payload = {
      enabled: true,
      provider: "openai",
      api_key: "k",
      model: "m",
      endpoint: "e",
      extra_headers: {},
      language: "en",
      max_tokens: 1,
      max_requests_per_minute: 1,
      include_process_tree: true,
      include_port_map: true,
    };
    await mutationFn(payload);

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/ai/config");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(payload));

    if (onSuccess) await Promise.resolve(onSuccess({ ok: true, config: payload }, payload, undefined));
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["ai-config"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["ai-status"] });
    expect(toastMock.success).toHaveBeenCalledWith("AI settings saved.");
  });
});

// ---------------------------------------------------------------------------
// useTelegramConfigMutation
// ---------------------------------------------------------------------------

describe("useTelegramConfigMutation", () => {
  it("POSTs the telegram config and invalidates telegram-config", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, config: {} }));
    vi.stubGlobal("fetch", fetchMock);

    api.useTelegramConfigMutation();
    const { mutationFn, onSuccess } = lastMutation();

    const payload = {
      enabled: true,
      bot_token: "t",
      allowed_chat_ids: [1],
      api_base_url: "https://api.telegram.org",
      poll_timeout_sec: 25,
      notify_on_critical: true,
      notification_mode: "high_value",
      notification_types: [],
      require_confirm: true,
      confirm_ttl_sec: 90,
    };
    await mutationFn(payload);

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/telegram/config");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(payload));

    if (onSuccess) await Promise.resolve(onSuccess({ ok: true, config: payload }, payload, undefined));
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["telegram-config"] });
    expect(toastMock.success).toHaveBeenCalledWith("Telegram settings saved.");
  });
});

// ---------------------------------------------------------------------------
// useTelegramTestMutation
// ---------------------------------------------------------------------------

describe("useTelegramTestMutation", () => {
  it("POSTs to /telegram/test with no body", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, message: "hi" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useTelegramTestMutation();
    const { mutationFn } = lastMutation();

    const result = await mutationFn();

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/telegram/test");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();
    expect(result).toEqual({ ok: true, message: "hi" });
  });
});

// ---------------------------------------------------------------------------
// useAITestMutation
// ---------------------------------------------------------------------------

describe("useAITestMutation", () => {
  it("POSTs to /ai/test with no body", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, response: "ok" }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAITestMutation();
    const { mutationFn } = lastMutation();

    const result = await mutationFn();

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/ai/test");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();
    expect(result).toEqual({ ok: true, response: "ok" });
  });
});

// ---------------------------------------------------------------------------
// useConfigUpdateMutation
// ---------------------------------------------------------------------------

describe("useConfigUpdateMutation", () => {
  it("PUTs the payload and invalidates config + info", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true, config: {} }));
    vi.stubGlobal("fetch", fetchMock);

    api.useConfigUpdateMutation();
    const { mutationFn, onSuccess } = lastMutation();

    const payload = { ui: { theme: "dark" } };
    await mutationFn(payload);

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/config");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(JSON.stringify(payload));
    expect(new Headers(init.headers).get("Content-Type")).toBe("application/json");

    if (onSuccess) await Promise.resolve(onSuccess({ ok: true, config: {} }, payload, undefined));
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["config"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["info"] });
    expect(toastMock.success).toHaveBeenCalledWith("Runtime settings saved.");
  });
});

// ---------------------------------------------------------------------------
// useProcessActionMutation
// ---------------------------------------------------------------------------

describe("useProcessActionMutation", () => {
  it("POSTs to /processes/<pid>/kill?confirm=true and invalidates system+alerts", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessActionMutation();
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn({ pid: 4242, action: "kill" });

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/processes/4242/kill?confirm=true");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();

    if (onSuccess) await Promise.resolve(onSuccess({ ok: true }, { pid: 4242, action: "kill" }, undefined));
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["system"] });
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["alerts"] });
    expect(toastMock.success).toHaveBeenCalledWith("Process action completed.");
  });

  it("supports the suspend action", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ pid: 11, action: "suspend" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/processes/11/suspend?confirm=true");
  });

  it("supports the resume action", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ pid: 22, action: "resume" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/processes/22/resume?confirm=true");
  });

  it("uses a custom success message when provided", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessActionMutation({ successMessage: "Killed it!" });
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn({ pid: 33, action: "kill" });
    if (onSuccess) await Promise.resolve(onSuccess({ ok: true }, { pid: 33, action: "kill" }, undefined));

    expect(toastMock.success).toHaveBeenCalledWith("Killed it!");
  });

  it("suppresses the success toast when successMessage is false", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useProcessActionMutation({ successMessage: false });
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn({ pid: 44, action: "kill" });
    if (onSuccess) await Promise.resolve(onSuccess({ ok: true }, { pid: 44, action: "kill" }, undefined));

    expect(toastMock.success).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// useAlertActionMutation
// ---------------------------------------------------------------------------

describe("useAlertActionMutation", () => {
  it("dismisses an alert with a pid and invalidates alerts", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertActionMutation();
    const { mutationFn, onSuccess } = lastMutation();

    await mutationFn({ type: "runaway_cpu", pid: 99, action: "dismiss" });

    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/api/v1/alerts/runaway_cpu/99/dismiss");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();

    if (onSuccess) {
      await Promise.resolve(onSuccess({ ok: true }, { type: "runaway_cpu", pid: 99, action: "dismiss" }, undefined));
    }
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ["alerts"] });
    expect(toastMock.success).toHaveBeenCalledWith("Alert state updated.");
  });

  it("snoozes an alert with a pid for 30m", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ type: "memory_leak", pid: 100, action: "snooze" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/alerts/memory_leak/100/snooze?duration=30m");
  });

  it("dismisses an alert without a pid", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ type: "system_alert", action: "dismiss" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/alerts/system_alert/dismiss");
  });

  it("snoozes an alert without a pid", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ type: "global_alert", action: "snooze" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/alerts/global_alert/snooze?duration=30m");
  });

  it("URL-encodes the alert type", async () => {
    const fetchMock = vi.fn(async (_input: string, _init: RequestInit) => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    api.useAlertActionMutation();
    const { mutationFn } = lastMutation();

    await mutationFn({ type: "rule:foo/bar", pid: 5, action: "dismiss" });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/alerts/rule%3Afoo%2Fbar/5/dismiss");
  });
});