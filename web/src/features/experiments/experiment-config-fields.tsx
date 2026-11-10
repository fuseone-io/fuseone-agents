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
import { useIntegrations } from "@/features/integrations/api";
import { AS_PUBLISHED, type ExperimentForm } from "@/features/experiments/experiment-form";

/**
 * What the experiment overrides, field by field: each one is the version's own
 * until somebody picks something else. A provider alone keeps the version's
 * model; a model alone keeps its provider.
 */
export function ExperimentConfigFields({
  form,
  publishedProvider,
}: {
  form: UseFormReturn<ExperimentForm>;
  publishedProvider: string;
}) {
  const { t } = useTranslation();
  const integrations = useIntegrations().data;
  const providers = integrations?.providers ?? [];
  const provider = useWatch({ control: form.control, name: "provider" });
  const effective = provider === AS_PUBLISHED ? publishedProvider : provider;

  return (
    <div className="space-y-4">
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
                {providers.map((p) => (
                  <SelectItem key={p.name} value={p.name} className="font-mono">
                    {p.name}
                  </SelectItem>
                ))}
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
              options={modelsFor(effective, providers, integrations?.presets ?? [])}
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
