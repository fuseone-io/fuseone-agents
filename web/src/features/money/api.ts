import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";
import { currentCurrency } from "@/lib/format";

export type MoneySettings = components["schemas"]["MoneySettings"];

export const moneyKeys = {
  all: ["money"] as const,
  current: () => [...moneyKeys.all, "current"] as const,
};

export function useMoney() {
  return useQuery({
    queryKey: moneyKeys.current(),
    queryFn: async () => unwrap(await api.GET("/money")),
    staleTime: 60_000,
    retry: 1,
  });
}

/**
 * The installation's currency, re-rendering when /money answers.
 *
 * currentCurrency() is a module global the provider sets during render, so a
 * component that read it before /money answered kept showing the default —
 * a ceiling labelled BRL on an installation that bills in USD.
 */
export function useInstallationCurrency(): string {
  return useMoney().data?.currency ?? currentCurrency();
}

export function useSetMoney() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (settings: MoneySettings) =>
      unwrap(await api.PUT("/admin/money", { body: settings })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: moneyKeys.all });
    },
  });
}
