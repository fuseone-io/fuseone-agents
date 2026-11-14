import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { useWatch } from "react-hook-form";
import { SuggestPopover } from "@/components/shared/suggest-popover";
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
import { useProviderChoices } from "@/features/agents/providers-api";

/**
 * Which model a rate is for, offered rather than dictated.
 *
 * The suggestions are the providers and models already priced plus the
 * connected providers and the models each serves (GET /providers — the
 * reduced listing, readable on tool:read like the prices themselves, where
 * the integrations listing is not). Typing stays valid: a rate may be set
 * before the model is ever used, and a refused listing costs only the hints.
 */
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
  const choices = useProviderChoices(undefined, !locked).data?.items ?? [];
  const choose = (name: "provider" | "model") => (value: string) =>
    form.setValue(name, value, { shouldDirty: true, shouldValidate: true });

  return (
    <>
      <FormField
        control={form.control}
        name="provider"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("agents.provider")}</FormLabel>
            <div className="flex min-w-0 items-center gap-1">
              <FormControl>
                <Input
                  {...field}
                  disabled={locked}
                  className="min-w-0 flex-1 font-mono"
                  placeholder="anthropic"
                />
              </FormControl>
              {!locked && (
                <SuggestPopover
                  label={t("admin.knownProviders")}
                  options={knownPriceProviders(suggestions, choices).map(
                    (value) => ({ value }),
                  )}
                  selected={field.value}
                  onPick={choose("provider")}
                />
              )}
            </div>
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="model"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.model")}</FormLabel>
            <div className="flex min-w-0 items-center gap-1">
              <FormControl>
                <Input
                  {...field}
                  disabled={locked}
                  className="min-w-0 flex-1 font-mono"
                  placeholder="claude-opus-5"
                />
              </FormControl>
              {!locked && (
                <SuggestPopover
                  label={t("admin.knownModels")}
                  options={knownPriceModels(suggestions, provider, choices).map(
                    (value) => ({ value }),
                  )}
                  selected={field.value}
                  onPick={choose("model")}
                />
              )}
            </div>
          </FormItem>
        )}
      />
    </>
  );
}
