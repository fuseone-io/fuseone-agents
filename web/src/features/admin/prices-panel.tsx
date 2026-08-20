import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CircleAlert, Coins, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { PriceCurrencyControl } from "@/features/admin/price-currency-control";
import { PriceForm } from "@/features/admin/price-form";
import { PriceRow } from "@/features/admin/price-row";
import { usePrices, type ModelPrice } from "@/features/admin/prices-api";
import { useVisibleItems } from "@/hooks/use-visible-items";

/**
 * What this installation pays per model.
 *
 * Market defaults are displayed as sourced reference values, not as accounting
 * rates. Until a custom rate exists in the installation's currency, Cost.Micros
 * remains zero and money ceilings cannot rely on a foreign unit.
 */
export function PricesPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = usePrices();
  const [editing, setEditing] = useState<ModelPrice | null | undefined>();
  const prices = data?.items ?? [];
  const page = useVisibleItems(prices, 50);

  return (
    <Panel
      title={t("admin.prices")}
      action={
        <div className="flex flex-wrap items-center justify-end gap-2">
          <PriceCurrencyControl />
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              void refetch().then(() =>
                toast.success(t("admin.marketDefaultsRefreshed")),
              )
            }
          >
            <RefreshCw className="size-4" aria-hidden />
            {t("admin.refreshMarketDefaults")}
          </Button>
          <Button size="sm" onClick={() => setEditing(null)}>
            <Plus className="size-4" aria-hidden />
            {t("common.new")}
          </Button>
        </div>
      }
    >
      <div className="mb-3 flex items-start gap-2 rounded-lg border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm text-amber-950 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
        <CircleAlert
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-300"
          aria-hidden
        />
        <p>{t("admin.currencyHistoryWarning")}</p>
      </div>

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : prices.length === 0 ? (
        <EmptyState
          icon={<Coins className="size-6" />}
          title={t("admin.noPrices")}
          hint={t("admin.noPricesHint")}
        />
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {page.visible.map((price) => (
              <PriceRow
                key={`${price.provider}/${price.model}`}
                price={price}
                onEdit={() => setEditing(price)}
              />
            ))}
          </ul>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}

      {editing !== undefined && (
        <PriceForm
          price={editing}
          knownPrices={prices}
          onClose={() => setEditing(undefined)}
        />
      )}
    </Panel>
  );
}
