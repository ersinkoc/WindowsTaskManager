import { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "./query-client";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
  },
}));

const mockedToast = vi.mocked(toast);

describe("queryClient", () => {
  beforeEach(() => {
    mockedToast.error.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("is an instance of QueryClient", () => {
    expect(queryClient).toBeInstanceOf(QueryClient);
  });

  it("applies the configured default options for queries", () => {
    const defaults = queryClient.getDefaultOptions();
    expect(defaults.queries?.staleTime).toBe(30_000);
    expect(defaults.queries?.retry).toBe(1);
    expect(defaults.queries?.refetchOnWindowFocus).toBe(false);
  });

  describe("QueryCache onError", () => {
    function getOnError(): (error: unknown, query: unknown) => void {
      // The QueryClient exposes the underlying caches via public accessors;
      // each cache keeps the original config (including the onError callback).
      const cache = queryClient.getQueryCache();
      const onError = cache.config.onError as
        | ((error: unknown, query: unknown) => void)
        | undefined;
      if (!onError) throw new Error("QueryCache onError not configured");
      return onError;
    }

    it("toasts the error message when the query already has data", () => {
      const onError = getOnError();
      const queryLike = { state: { data: { ok: true } } };

      onError(new Error("boom"), queryLike);

      expect(mockedToast.error).toHaveBeenCalledWith("boom");
    });

    it("does not toast when the query has no data yet", () => {
      const onError = getOnError();
      const queryLike = { state: { data: undefined } };

      onError(new Error("hidden"), queryLike);

      expect(mockedToast.error).not.toHaveBeenCalled();
    });

    it("uses the fallback message for a non-Error throwable when data is present", () => {
      const onError = getOnError();
      const queryLike = { state: { data: { ok: true } } };

      onError("plain string", queryLike);

      expect(mockedToast.error).toHaveBeenCalledWith(
        "The request could not be completed.",
      );
    });
  });

  describe("MutationCache onError", () => {
    function getOnError(): (error: unknown) => void {
      const cache = queryClient.getMutationCache();
      const onError = cache.config.onError as
        | ((error: unknown) => void)
        | undefined;
      if (!onError) throw new Error("MutationCache onError not configured");
      return onError;
    }

    it("toasts the error message for an Error", () => {
      const onError = getOnError();
      onError(new Error("mutation failed"));
      expect(mockedToast.error).toHaveBeenCalledWith("mutation failed");
    });

    it("uses the fallback message for non-Error throwables", () => {
      const onError = getOnError();
      onError("plain string");
      expect(mockedToast.error).toHaveBeenCalledWith(
        "The request could not be completed.",
      );
    });

    it("uses the fallback message for an Error without a message", () => {
      const onError = getOnError();
      onError(new Error(""));
      expect(mockedToast.error).toHaveBeenCalledWith(
        "The request could not be completed.",
      );
    });
  });
});
