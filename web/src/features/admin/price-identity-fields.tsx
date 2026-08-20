import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { useWatch } from "react-hook-form";
import { Input } from "@/components/ui/input";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import {
  knownPriceModels,
  knownPriceProviders,
  type PriceFormValues,
  type PriceSuggestion,
} from "@/features/admin/price-form-model";
import { PriceSuggestionButtons } from "@/features/admin/price-suggestion-buttons";

export function PriceIdentityFields({
  form,
  locked,
  suggestions,
}: {
  form: UseFormReturn<PriceFormValues>;
  locked: boolean;
  suggestions: PriceSuggestion[];
}) {
  const { t } = useTranslation();
  const provider = useWatch({ control: form.control, name: "provider" });
  const providers = knownPriceProviders(suggestions);
  const models = knownPriceModels(suggestions, provider);

  return (
    <>
      <FormField
        control={form.control}
        name="provider"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("agents.provider")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                disabled={locked}
                className="font-mono"
                placeholder="anthropic"
              />
            </FormControl>
            <PriceSuggestionButtons
              label={t("admin.knownProviders")}
              options={providers}
              disabled={locked}
              onChoose={(value) =>
                form.setValue("provider", value, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="model"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.model")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                disabled={locked}
                className="font-mono"
                placeholder="claude-opus-5"
              />
            </FormControl>
            <PriceSuggestionButtons
              label={t("admin.knownModels")}
              options={models}
              disabled={locked}
              onChoose={(value) =>
                form.setValue("model", value, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            />
          </FormItem>
        )}
      />
    </>
  );
}
