import { useTranslation } from "react-i18next";
import { useWatch, type UseFormReturn } from "react-hook-form";
import { FormControl, FormDescription, FormField, FormItem, FormLabel } from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ModelField, modelsFor } from "@/features/agents/model-field";
import { ProviderOptions } from "@/features/agents/provider-options";
import { useProviderChoices } from "@/features/agents/providers-api";
import { TargetError } from "@/features/agents/target-error";
import { AS_PUBLISHED, type ExperimentForm } from "@/features/experiments/experiment-form";

/**
 * What the experiment overrides, field by field: each one is the version's own
 * until somebody picks something else. A provider alone keeps the version's
 * model; a model alone keeps its provider.
 */
export function ExperimentConfigFields({
  form,
  publishedProvider,
  target,
}: {
  form: UseFormReturn<ExperimentForm>;
  publishedProvider: string;
  /** The agent's scope, which the providers are asked for (NT-019). */
  target?: string;
}) {
  const { t } = useTranslation();
  const choices = useProviderChoices(target, target !== undefined);
  const providers = choices.data?.items ?? [];
  const provider = useWatch({ control: form.control, name: "provider" });
  const effective = provider === AS_PUBLISHED ? publishedProvider : provider;

  return (
    <div className="space-y-4">
      {choices.error && (
        <TargetError error={choices.error} onRetry={() => void choices.refetch()} />
      )}
      <FormField
        control={form.control}
        name="provider"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("experiments.provider")}</FormLabel>
            <Select
              value={field.value}
              onValueChange={(next) => {
                field.onChange(next);
                form.setValue("model", "");
              }}
            >
              <FormControl>
                <SelectTrigger className="w-full font-mono">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value={AS_PUBLISHED}>{t("experiments.asPublished")}</SelectItem>
                <ProviderOptions providers={providers} current={field.value} />
              </SelectContent>
            </Select>
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="model"
        render={({ field }) => (
          <FormItem>
            <FormLabel htmlFor="experiment-model">{t("experiments.model")}</FormLabel>
            <ModelField
              id="experiment-model"
              value={field.value}
              options={modelsFor(effective, providers, [])}
              onChange={field.onChange}
            />
            <FormDescription>{t("experiments.modelHint")}</FormDescription>
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="effort"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("experiments.effort")}</FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full font-mono">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value={AS_PUBLISHED}>{t("experiments.asPublished")}</SelectItem>
                {["low", "medium", "high"].map((level) => (
                  <SelectItem key={level} value={level} className="font-mono">
                    {level}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>{t("experiments.noFallback")}</FormDescription>
          </FormItem>
        )}
      />
    </div>
  );
}
