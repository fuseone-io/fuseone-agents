import { useTranslation } from "react-i18next";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { FlaskConical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ExperimentCost } from "@/features/experiments/experiment-cost";
import { useRunAgainstCases, type CaseRun } from "@/features/workbench/case-run-api";
import { formatInstant } from "@/lib/format";
import { problemMessage } from "@/lib/api/problem-message";
import type { CandidateSeal } from "@/lib/api/client";

const schema = z.object({
  sealId: z.string().min(1, "workbench.experiment.candidateRequired"),
  repetitions: z.enum(["1", "2", "3", "4", "5"]),
});
type Values = z.infer<typeof schema>;

/** Choosing the candidate and how many times each case runs, with what that costs. */
export function CaseRunStart({ agentId, candidates, onOpened }: { agentId: string; candidates: CandidateSeal[]; onOpened: (run: CaseRun) => void }) {
  const { t } = useTranslation();
  const run = useRunAgainstCases(agentId);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { sealId: candidates[0]?.sealId ?? "", repetitions: "1" } });
  const repetitions = useWatch({ control: form.control, name: "repetitions" });

  const submit = (values: Values) =>
    run.mutate(
      { sealId: values.sealId, repetitions: Number(values.repetitions) },
      {
        onSuccess: (opened) => {
          if (opened.baseline) toast.success(t(opened.reused ? "workbench.experiment.reused" : "workbench.experiment.opened", { id: opened.baseline }));
          onOpened(opened);
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} className="flex flex-col gap-4">
        <div className="flex flex-wrap gap-4">
          <FormField
            control={form.control}
            name="sealId"
            render={({ field }) => (
              <FormItem className="min-w-0">
                <FormLabel>{t("workbench.rehearsal.candidate")}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger className="w-full min-w-0 sm:w-auto sm:min-w-80"><SelectValue className="min-w-0 truncate" /></SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {candidates.map((c) => (
                      <SelectItem key={c.sealId} value={c.sealId}>
                        {t("workbench.rehearsal.candidateOption", { version: c.version, sealedAt: formatInstant(c.sealedAt) })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="repetitions"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("experiments.repetitions")}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {schema.shape.repetitions.options.map((n) => <SelectItem key={n} value={n}>{n}</SelectItem>)}
                  </SelectContent>
                </Select>
              </FormItem>
            )}
          />
        </div>
        <ExperimentCost agentId={agentId} repetitions={Number(repetitions)} candidate />
        <p className="max-w-prose text-xs text-muted-foreground">{t("workbench.experiment.baselineNote")}</p>
        <Button type="submit" variant="outline" className="self-start" disabled={run.isPending}>
          <FlaskConical className="size-4" aria-hidden />
          {t("workbench.experiment.run")}
        </Button>
      </form>
    </Form>
  );
}
