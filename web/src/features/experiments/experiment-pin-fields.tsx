import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel } from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ExperimentForm } from "@/features/experiments/experiment-form";
import type { AgentVersion } from "@/lib/api/client";

/** Which version runs: the current one by default, an older one when chosen. */
export function VersionField({
  form,
  versions,
}: {
  form: UseFormReturn<ExperimentForm>;
  versions: AgentVersion[];
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name="version"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("experiments.agentVersion")}</FormLabel>
          <Select value={field.value} onValueChange={field.onChange}>
            <FormControl>
              <SelectTrigger className="w-full font-mono">
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {versions.map((v) => (
                <SelectItem key={v.versionId} value={v.versionId} className="font-mono">
                  {v.versionId.slice(0, 9)}
                  {v.latest ? ` · ${t("experiments.current")}` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormItem>
      )}
    />
  );
}

/** How many times each case runs, 1 to 5: each repetition is its own run. */
export function RepetitionsField({ form }: { form: UseFormReturn<ExperimentForm> }) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name="repetitions"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("experiments.repetitions")}</FormLabel>
          <Select value={field.value} onValueChange={field.onChange}>
            <FormControl>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {["1", "2", "3", "4", "5"].map((n) => (
                <SelectItem key={n} value={n}>{n}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormItem>
      )}
    />
  );
}
