import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./api-client";

export interface HistoryDataPoint {
  time: string;
  cpu_total: number;
  memory_used_percent: number;
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
