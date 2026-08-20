import { useTranslation } from "react-i18next";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, useWatch } from "react-hook-form";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { Form } from "@/components/ui/form";
import { usePutPrice, type ModelPrice } from "@/features/admin/prices-api";
import {
  initialPriceFormValues,
  modelPriceFromForm,
  PRICE_RATES,
  priceFormSchema,
  type PriceSuggestion,
  type PriceFormValues,
} from "@/features/admin/price-form-model";
import { PriceIdentityFields } from "@/features/admin/price-identity-fields";
import { PriceRateField } from "@/features/admin/price-rate-field";

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
  knownPrices,
  onClose,
}: {
  price: ModelPrice | null;
  knownPrices: PriceSuggestion[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutPrice();
  const isMarketDefault = price?.source === "market_default";
  const form = useForm<PriceFormValues>({
    resolver: zodResolver(priceFormSchema),
    mode: "onChange",
    defaultValues: initialPriceFormValues(price),
  });
  const values = useWatch({ control: form.control });
  const canSave = priceFormSchema.safeParse(values).success && !put.isPending;

  const submit = (draft: PriceFormValues) =>
    put.mutate(modelPriceFromForm(draft), {
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
      title={t(
        price
          ? isMarketDefault
            ? "admin.overrideMarketRate"
            : "admin.editRate"
          : "admin.newRate",
      )}
      description={t(
        isMarketDefault ? "admin.marketRateOverrideHint" : "admin.ratesAreYours",
      )}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody>
            <div className="grid gap-3 sm:grid-cols-2">
              <PriceIdentityFields
                form={form}
                locked={!!price}
                suggestions={knownPrices}
              />

              {PRICE_RATES.map(({ field, label }) => (
                <PriceRateField
                  key={field}
                  control={form.control}
                  name={field}
                  label={t(label)}
                />
              ))}
            </div>
          </PropertiesSheetBody>

          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!canSave}>
              {t("common.save")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
