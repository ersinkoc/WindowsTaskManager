import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useLocalStorage } from "./use-local-storage";

describe("useLocalStorage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it("returns the initial value when localStorage has no entry", () => {
    const { result } = renderHook(() => useLocalStorage<number>("missing-key", 42));
    expect(result.current[0]).toBe(42);
  });

  it("returns the parsed stored value when localStorage has a JSON entry", () => {
    window.localStorage.setItem("present-key", JSON.stringify({ a: 1 }));
    const { result } = renderHook(() => useLocalStorage<{ a: number }>("present-key", { a: 0 }));
    expect(result.current[0]).toEqual({ a: 1 });
  });

  it("falls back to the initial value when the stored value is invalid JSON", () => {
    window.localStorage.setItem("broken-key", "{not-json");
    const { result } = renderHook(() => useLocalStorage<{ a: number }>("broken-key", { a: 7 }));
    expect(result.current[0]).toEqual({ a: 7 });
  });

  it("persists the value to localStorage after it changes", () => {
    const { result } = renderHook(() => useLocalStorage<string>("persist-key", "init"));

    act(() => {
      result.current[1]("next");
    });

    expect(result.current[0]).toBe("next");
    expect(window.localStorage.getItem("persist-key")).toBe(JSON.stringify("next"));
  });

  it("persists updates for object values", () => {
    type Shape = { count: number };
    const { result } = renderHook(() => useLocalStorage<Shape>("obj-key", { count: 0 }));

    act(() => {
      result.current[1]({ count: 3 });
    });

    expect(result.current[0]).toEqual({ count: 3 });
    expect(window.localStorage.getItem("obj-key")).toBe(JSON.stringify({ count: 3 }));
  });

  it("re-persists when the storage key changes", () => {
    const { result, rerender } = renderHook(
      ({ k }: { k: string }) => useLocalStorage<string>(k, "init"),
      { initialProps: { k: "key-a" } },
    );

    act(() => {
      result.current[1]("value-a");
    });

    expect(window.localStorage.getItem("key-a")).toBe(JSON.stringify("value-a"));

    rerender({ k: "key-b" });

    // Changing the key causes the hook to initialize from "key-b" (missing),
    // then re-write the current value under "key-b".
    expect(window.localStorage.getItem("key-b")).toBe(JSON.stringify("value-a"));
  });
});
