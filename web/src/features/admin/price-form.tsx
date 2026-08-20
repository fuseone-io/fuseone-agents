import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Labelled } from "@/features/policies/section";
import { usePutPrice, type ModelPrice } from "@/features/admin/prices-api";

const RATES = [
  { field: "inputMicros", label: "admin.rateInput" },
  { field: "outputMicros", label: "admin.rateOutput" },
  { field: "cacheReadMicros", label: "admin.rateCacheRead" },
  { field: "cacheWriteMicros", label: "admin.rateCacheWrite" },
] as const;

/**
 * A rate, in the installation's currency per million tokens.
 *
 * Four fields rather than one. A cache read costs a fraction of an input
 * token, and collapsing them into a single number is what makes an agent's
 * cost impossible to diagnose (PRD FO-08) — the figure would be right in
 * total and useless for finding which agent is expensive and why.
 */
export function PriceForm({
  price,
  onClose,
}: {
  price: ModelPrice | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutPrice();
  const [draft, setDraft] = useState<ModelPrice>(
    price ?? { provider: "", model: "", inputMicros: 0, outputMicros: 0 },
  );

  const submit = () =>
    put.mutate(draft, {
      onSuccess: () => {
        toast.success(t("admin.rateSet"), {
          description: t("admin.rateApplies"),
        });
        onClose();
      },
      onError: (e) =>
        toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
    });

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t(price ? "admin.editRate" : "admin.newRate")}
      description={t("admin.ratesAreYours")}
    >
      <div className="flex min-h-0 flex-1 flex-col">
        <PropertiesSheetBody>
        <div className="grid gap-3 sm:grid-cols-2">
          <Labelled label={t("agents.provider")} htmlFor="price-provider">
            <Input
              id="price-provider"
              value={draft.provider}
              disabled={!!price}
              onChange={(e) => setDraft({ ...draft, provider: e.target.value })}
              className="font-mono"
              placeholder="anthropic"
            />
          </Labelled>
          <Labelled label={t("admin.model")} htmlFor="price-model">
            <Input
              id="price-model"
              value={draft.model}
              disabled={!!price}
              onChange={(e) => setDraft({ ...draft, model: e.target.value })}
              className="font-mono"
              placeholder="claude-opus-5"
            />
          </Labelled>

          {RATES.map(({ field, label }) => (
            <Labelled key={field} label={t(label)} htmlFor={`price-${field}`}>
              <Input
                id={`price-${field}`}
                inputMode="decimal"
                value={
                  draft[field] ? String((draft[field] ?? 0) / 1_000_000) : ""
                }
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    [field]:
                      Math.round(
                        Number(e.target.value.replace(",", ".")) * 1_000_000,
                      ) || 0,
                  })
                }
                className="font-mono"
                placeholder="0"
              />
            </Labelled>
          ))}
        </div>
        </PropertiesSheetBody>

        <PropertiesSheetFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!draft.provider || !draft.model || put.isPending}
            onClick={submit}
          >
            {t("common.save")}
          </Button>
        </PropertiesSheetFooter>
      </div>
    </PropertiesSheet>
  );
}
