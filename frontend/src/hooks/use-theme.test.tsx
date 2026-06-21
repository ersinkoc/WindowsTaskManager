import { act, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ThemeProvider, useTheme } from "./use-theme";

type Listener = (event: { matches: boolean }) => void;

interface MockMediaQueryList {
  matches: boolean;
  media: string;
  onchange: null;
  addEventListener: (event: string, cb: Listener) => void;
  removeEventListener: (event: string, cb: Listener) => void;
  addListener: () => void;
  removeListener: () => void;
  dispatchEvent: () => boolean;
  __listeners: Set<Listener>;
  __trigger: (matches: boolean) => void;
}

function installMatchMedia(initialMatches: boolean) {
  const listeners = new Set<Listener>();
  const list: MockMediaQueryList = {
    matches: initialMatches,
    media: "(prefers-color-scheme: dark)",
    onchange: null,
    addEventListener: (_event: string, cb: Listener) => {
      listeners.add(cb);
    },
    removeEventListener: (_event: string, cb: Listener) => {
      listeners.delete(cb);
    },
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
    __listeners: listeners,
    __trigger: (matches: boolean) => {
      list.matches = matches;
      for (const cb of listeners) cb({ matches });
    },
  };
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: () => list,
  });
  return list;
}

describe("ThemeProvider", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.removeAttribute("data-theme-preference");
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it("exposes the context value and resolves 'system' to light when matchMedia is light", () => {
    installMatchMedia(false);
    const { result } = renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    });

    expect(result.current.theme).toBe("system");
    expect(result.current.resolvedTheme).toBe("light");
    expect(typeof result.current.setTheme).toBe("function");
  });

  it("resolves 'system' to dark when matchMedia prefers dark", () => {
    installMatchMedia(true);
    const { result } = renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    });
    expect(result.current.resolvedTheme).toBe("dark");
  });

  it("keeps an explicit 'dark' theme dark regardless of system preference", () => {
    installMatchMedia(false);
    window.localStorage.setItem("wtm-theme", JSON.stringify("dark"));

    const { result } = renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    });

    expect(result.current.theme).toBe("dark");
    expect(result.current.resolvedTheme).toBe("dark");
  });

  it("keeps an explicit 'light' theme light regardless of system preference", () => {
    installMatchMedia(true);
    window.localStorage.setItem("wtm-theme", JSON.stringify("light"));

    const { result } = renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    });

    expect(result.current.theme).toBe("light");
    expect(result.current.resolvedTheme).toBe("light");
  });

  it("writes the resolved theme and the preference to the document dataset", () => {
    installMatchMedia(false);
    render(
      <ThemeProvider>
        <span>child</span>
      </ThemeProvider>,
    );

    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.documentElement.dataset.themePreference).toBe("system");
  });

  it("updates the document dataset when the theme changes", () => {
    installMatchMedia(false);
    const { result } = renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    });

    act(() => {
      result.current.setTheme("dark");
    });

    expect(result.current.theme).toBe("dark");
    expect(result.current.resolvedTheme).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(document.documentElement.dataset.themePreference).toBe("dark");
  });

  it("updates document.dataset.theme when system preference changes (theme === 'system')", () => {
    const mql = installMatchMedia(false);
    render(
      <ThemeProvider>
        <span>child</span>
      </ThemeProvider>,
    );

    expect(document.documentElement.dataset.theme).toBe("light");

    act(() => {
      mql.__trigger(true);
    });
    expect(document.documentElement.dataset.theme).toBe("dark");

    act(() => {
      mql.__trigger(false);
    });
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("does not change document.dataset.theme when system preference changes if theme is not 'system'", () => {
    const mql = installMatchMedia(false);
    window.localStorage.setItem("wtm-theme", JSON.stringify("dark"));
    render(
      <ThemeProvider>
        <span>child</span>
      </ThemeProvider>,
    );

    expect(document.documentElement.dataset.theme).toBe("dark");

    act(() => {
      mql.__trigger(true);
    });

    // Listener short-circuits when theme !== 'system'
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("throws when useTheme is used outside ThemeProvider", () => {
    // Suppress the React error boundary log for this expected error.
    const consoleError = console.error;
    console.error = () => undefined;

    expect(() => renderHook(() => useTheme())).toThrow(
      "useTheme must be used inside ThemeProvider",
    );

    console.error = consoleError;
  });

  it("renders children inside the provider", () => {
    installMatchMedia(false);
    render(
      <ThemeProvider>
        <span>hello-theme</span>
      </ThemeProvider>,
    );
    expect(screen.getByText("hello-theme")).toBeInTheDocument();
  });
});
