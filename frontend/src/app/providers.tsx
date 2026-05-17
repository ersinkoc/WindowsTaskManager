import type { PropsWithChildren } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "../hooks/use-theme";
import { queryClient } from "../lib/query-client";
import { LiveDataBridge } from "./live-data-bridge";
import { WebVitals } from "../components/shared/web-vitals";

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <LiveDataBridge />
        <WebVitals />
        {children}
      </ThemeProvider>
    </QueryClientProvider>
  );
}
