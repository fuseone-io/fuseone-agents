import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Coins, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { RemoveButton } from "@/components/shared/remove-button";
import { PriceForm } from "@/features/admin/price-form";
import {
  useDeletePrice,
  usePrices,
  type ModelPrice,
} from "@/features/admin/prices-api";
import { useVisibleItems } from "@/hooks/use-visible-items";
import { formatMicros } from "@/lib/format";
import { currentLocale } from "@/i18n";

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
        <div className="flex items-center gap-2">
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
        <PriceForm price={editing} onClose={() => setEditing(undefined)} />
      )}
    </Panel>
  );
}

function PriceRow({
  price,
  onEdit,
}: {
  price: ModelPrice;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const remove = useDeletePrice();
  const isMarketDefault = price.source === "market_default";

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <button
        type="button"
        onClick={onEdit}
        className="min-w-0 flex-1 text-left focus-visible:underline focus-visible:outline-none"
      >
        <div className="flex flex-wrap items-center gap-2">
          <div className="font-mono text-sm">
            {price.provider}/{price.model}
          </div>
          <Badge variant={isMarketDefault ? "outline" : "secondary"}>
            {t(isMarketDefault ? "admin.marketDefault" : "admin.customRate")}
          </Badge>
        </div>
        {/* All four, always. Reading only the input rate is how somebody
            concludes a cached agent is expensive. */}
        <Mono dim>
          {t("admin.rateLine", {
            input: formatPrice(price, price.inputMicros ?? 0),
            output: formatPrice(price, price.outputMicros ?? 0),
            cacheRead: formatPrice(price, price.cacheReadMicros ?? 0),
            cacheWrite: formatPrice(price, price.cacheWriteMicros ?? 0),
          })}
        </Mono>
        {isMarketDefault && price.sourceUpdatedAt && (
          <div className="mt-1 text-xs text-muted-foreground">
            {t("admin.marketDefaultChecked", {
              date: price.sourceUpdatedAt,
              currency: price.currency ?? "",
            })}
          </div>
        )}
      </button>
      {!isMarketDefault && (
        <RemoveButton
          title={t("admin.removeRate", { model: price.model })}
          description={t("admin.removeRateHint")}
          onConfirm={() =>
            remove.mutate(
              { provider: price.provider, model: price.model },
              {
                onSuccess: () => toast.success(t("admin.rateRemoved")),
                onError: (e) =>
                  toast.error(
                    e instanceof Error ? e.message : t("common.removeFailed"),
                  ),
              },
            )
          }
        />
      )}
    </li>
  );
}

function formatPrice(price: ModelPrice, micros: number): string {
  if (price.source !== "market_default" || !price.currency) {
    return formatMicros(micros);
  }
  const value = micros / 1_000_000;
  const digits =
    value !== 0 && Math.abs(value) < 0.01 ? { maximumFractionDigits: 4 } : {};
  return new Intl.NumberFormat(currentLocale(), {
    style: "currency",
    currency: price.currency,
    ...digits,
  }).format(value);
}
