import { useEffect, type ReactNode } from "react";
import {
  DEFAULT_CURRENCY,
  currentCurrency,
  setInstallationCurrency,
} from "@/lib/format";
import { useMoney } from "@/features/money/api";

export function MoneyProvider({ children }: { children: ReactNode }) {
  const { data } = useMoney();
  const currency = data?.currency ?? currentCurrency();

  setInstallationCurrency(currency);

  useEffect(() => {
    return () => setInstallationCurrency(DEFAULT_CURRENCY);
  }, []);

  return <>{children}</>;
}
