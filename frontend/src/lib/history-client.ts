import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./api-client";

export interface HistoryDataPoint {
  time: string;
  cpu: { total_percent: number };
  memory: { used_percent: number };
  network: {
    total_down_bps: number;
    total_up_bps: number;
    interfaces: Array<{ name: string; in_bps: number; out_bps: number }>;
  };
}

export interface SystemHistoryResponse {
  history: HistoryDataPoint[];
}

export function useSystemHistoryQuery(seconds = 60) {
  return useQuery({
    queryKey: ["history", seconds],
    queryFn: () =>
      apiGet<SystemHistoryResponse>("/history", {
        params: { since: Math.floor(Date.now() / 1000 - seconds).toString() },
      }),
    staleTime: 1_000,
    refetchInterval: 1_000,
  });
}
