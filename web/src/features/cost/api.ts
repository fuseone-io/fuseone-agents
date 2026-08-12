import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

const DAY_MS = 24 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;

export const costKeys = {
  all: ["cost"] as const,
  rollup: (from: string, to: string, groupBy: string) =>
    [...costKeys.all, groupBy, from, to] as const,
};

/**
 * The window, computed once and rounded to the hour.
 *
 * Reading the clock during render produced a new bound on every pass, so the
 * query key never repeated and the page refetched continuously.
 */
export function useCostWindow(days = 30) {
  return useState(() => {
    const to = Math.floor(Date.now() / HOUR_MS) * HOUR_MS;
    return { from: new Date(to - days * DAY_MS).toISOString(), to: new Date(to).toISOString() };
  })[0];
}

export function useCostRollup(from: string, to: string, groupBy: "agent" | "area" | "company" | "day") {
  return useQuery({
    queryKey: costKeys.rollup(from, to, groupBy),
    queryFn: async () => unwrap(await api.GET("/cost", { params: { query: { from, to, groupBy } } })),
  });
}
