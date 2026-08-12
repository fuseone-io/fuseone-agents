import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Coins, Plus } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { RemoveButton } from "@/components/shared/remove-button";
import { PriceForm } from "@/features/admin/price-form";
import {
  useDeletePrice,
  usePrices,
  type ModelPrice,
} from "@/features/admin/prices-api";
import { formatMicros } from "@/lib/format";

/**
 * What this installation pays per model.
 *
 * Until a rate exists here, every cost in the console reads zero: the platform
 * counts tokens and refuses to guess at money, so a run's cost, an agent's
 * ceiling and the authoring assistant's daily bound are all decoration. That
 * is the honest failure, but it is a failure, and this screen is the only
 * thing that ends it.
 */
export function PricesPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = usePrices();
  const [editing, setEditing] = useState<ModelPrice | null | undefined>();
  const prices = data?.items ?? [];

  return (
    <Panel
      title={t("admin.prices")}
      action={
        <Button size="sm" onClick={() => setEditing(null)}>
          <Plus className="size-4" aria-hidden />
          {t("common.new")}
        </Button>
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
        <ul className="flex flex-col gap-2">
          {prices.map((price) => (
            <PriceRow
              key={`${price.provider}/${price.model}`}
              price={price}
              onEdit={() => setEditing(price)}
            />
          ))}
        </ul>
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

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <button
        type="button"
        onClick={onEdit}
        className="min-w-0 flex-1 text-left focus-visible:underline focus-visible:outline-none"
      >
        <div className="font-mono text-sm">
          {price.provider}/{price.model}
        </div>
        {/* All four, always. Reading only the input rate is how somebody
            concludes a cached agent is expensive. */}
        <Mono dim>
          {t("admin.rateLine", {
            input: formatMicros(price.inputMicros ?? 0),
            output: formatMicros(price.outputMicros ?? 0),
            cacheRead: formatMicros(price.cacheReadMicros ?? 0),
            cacheWrite: formatMicros(price.cacheWriteMicros ?? 0),
          })}
        </Mono>
      </button>
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
    </li>
  );
}
