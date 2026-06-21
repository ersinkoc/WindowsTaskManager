import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useDebouncedValue } from "./use-debounced-value";

describe("useDebouncedValue", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns the initial value on the first render (no delay applied yet)", () => {
    const { result } = renderHook(() => useDebouncedValue("hello", 300));
    expect(result.current).toBe("hello");
  });

  it("uses the default 300ms delay when no delay is provided", () => {
    const { result, rerender } = renderHook(({ value }: { value: string }) => useDebouncedValue(value), {
      initialProps: { value: "initial" },
    });
    expect(result.current).toBe("initial");

    rerender({ value: "updated" });
    // Not yet — still 300ms away from the debounce flush.
    expect(result.current).toBe("initial");

    act(() => {
      vi.advanceTimersByTime(299);
    });
    expect(result.current).toBe("initial");

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe("updated");
  });

  it("debounces value updates with the custom delay", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) => useDebouncedValue(value, 500),
      { initialProps: { value: "a" } },
    );
    expect(result.current).toBe("a");

    rerender({ value: "b" });
    expect(result.current).toBe("a");

    act(() => {
      vi.advanceTimersByTime(499);
    });
    expect(result.current).toBe("a");

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe("b");
  });

  it("collapses rapid successive updates into a single update after the delay", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: number }) => useDebouncedValue(value, 200),
      { initialProps: { value: 0 } },
    );
    expect(result.current).toBe(0);

    rerender({ value: 1 });
    act(() => {
      vi.advanceTimersByTime(50);
    });
    rerender({ value: 2 });
    act(() => {
      vi.advanceTimersByTime(50);
    });
    rerender({ value: 3 });
    act(() => {
      vi.advanceTimersByTime(50);
    });
    rerender({ value: 4 });

    // None of the intermediate values have flushed yet.
    expect(result.current).toBe(0);

    // The most-recent value should land once the trailing delay elapses.
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe(4);
  });

  it("clears the previous timer when value changes (no stale updates)", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) => useDebouncedValue(value, 100),
      { initialProps: { value: "first" } },
    );

    rerender({ value: "second" });
    act(() => {
      vi.advanceTimersByTime(50);
    });
    expect(result.current).toBe("first");

    // Change again — the first scheduled timeout must be cancelled.
    rerender({ value: "third" });
    act(() => {
      vi.advanceTimersByTime(50);
    });
    // Only 50ms after the second change, the first timer (which would have
    // fired at 100ms after the second render) should not have committed.
    expect(result.current).toBe("first");

    act(() => {
      vi.advanceTimersByTime(50);
    });
    expect(result.current).toBe("third");
  });

  it("clears the timer on unmount", () => {
    const clearTimeoutSpy = vi.spyOn(window, "clearTimeout");
    const { rerender, unmount } = renderHook(
      ({ value }: { value: string }) => useDebouncedValue(value, 100),
      { initialProps: { value: "x" } },
    );
    rerender({ value: "y" });
    unmount();
    expect(clearTimeoutSpy).toHaveBeenCalled();
  });

  it("supports non-string generic values (numbers)", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: number }) => useDebouncedValue<number>(value, 200),
      { initialProps: { value: 1 } },
    );
    expect(result.current).toBe(1);

    rerender({ value: 99 });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe(99);
  });

  it("supports object values (referential identity preserved across renders)", () => {
    const initial = { foo: 1 };
    const next = { foo: 2 };
    const { result, rerender } = renderHook(
      ({ value }: { value: { foo: number } }) => useDebouncedValue(value, 200),
      { initialProps: { value: initial } },
    );
    expect(result.current).toBe(initial);

    rerender({ value: next });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe(next);
  });

  it("re-runs the debounce window when the delay changes", () => {
    const { result, rerender } = renderHook(
      ({ value, delay }: { value: string; delay: number }) => useDebouncedValue(value, delay),
      { initialProps: { value: "a", delay: 100 } },
    );
    expect(result.current).toBe("a");

    rerender({ value: "b", delay: 100 });
    rerender({ value: "b", delay: 500 });
    act(() => {
      vi.advanceTimersByTime(100);
    });
    // 100ms is less than the new 500ms delay — still pending.
    expect(result.current).toBe("a");
    act(() => {
      vi.advanceTimersByTime(400);
    });
    expect(result.current).toBe("b");
  });
});
