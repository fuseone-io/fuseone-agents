import { useState } from "react";
import { useServiceTokens } from "@/features/admin/service-accounts-api";
import { liveTokens } from "@/features/admin/service-token-state";

export type EnableImpact =
  | { state: "reading" }
  | { state: "failed"; error: unknown }
  | { state: "read"; back: number };

/**
 * How many tokens enabling an account would give back, read when it is asked.
 *
 * Not the list on screen: that can be loading, refused or older than somebody
 * else's token, and treating any of those as a number would ask a person to
 * confirm a figure that is not true. Until a read succeeds there is no number.
 */
export function useEnableImpact(account: string) {
  const { refetch } = useServiceTokens(account);
  const [impact, setImpact] = useState<EnableImpact>({ state: "reading" });

  const readNow = async () => {
    setImpact({ state: "reading" });
    const result = await refetch();
    // A read overtaken by a newer one needs no guard: TanStack settles the older
    // promise with the newer fetch, so both give the same answer.
    setImpact(
      result.isSuccess
        ? { state: "read", back: liveTokens(result.data.items).length }
        : { state: "failed", error: result.error },
    );
  };

  return { impact, readNow };
}
