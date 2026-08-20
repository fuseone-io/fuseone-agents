import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { RemoveButton } from "@/components/shared/remove-button";
import {
  useDeletePrice,
  type ModelPrice,
} from "@/features/admin/prices-api";
import { formatCurrencyMicros, formatMicros } from "@/lib/format";

export function PriceRow({
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
  return formatCurrencyMicros(micros, price.currency);
}
