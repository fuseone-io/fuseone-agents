import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { FlaskConical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useCandidates } from "@/features/workbench/candidates-api";
import { useOpenRehearsal } from "@/features/workbench/rehearsal-api";
import { fitsIn } from "@/features/workbench/rehearsal-text";
import { formatInstant } from "@/lib/format";
import { problemMessage } from "@/lib/api/problem-message";
import type { CandidateSeal } from "@/lib/api/client";

/** The server's bound on a case: a message, not a dataset. */
const MAX_CASE_BYTES = 32 * 1024;

const schema = z.object({
  sealId: z.string().min(1, "workbench.rehearsal.candidateRequired"),
  text: z
    .string()
    .refine((s) => s.trim() !== "", "workbench.rehearsal.caseRequired")
    .refine(fitsIn(MAX_CASE_BYTES), "workbench.rehearsal.caseTooLong"),
});
type Values = z.infer<typeof schema>;

/** Choosing a candidate and writing the case a rehearsal opens on. */
export function RehearsalStart({ agentId, onOpened }: { agentId: string; onOpened: (runId: string) => void }) {
  const { t } = useTranslation();
  const candidates = useCandidates(agentId);

  if (candidates.isLoading) {
    return (
      <div role="status" aria-label={t("workbench.rehearsal.reading")} className="flex flex-col gap-2">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  if (candidates.error) {
    return <LoadFailed message={problemMessage(candidates.error, t)} onRetry={() => void candidates.refetch()} />;
  }
  if (!candidates.data?.length) {
    return (
      <div className="flex flex-col gap-1 rounded-lg border border-dashed p-4">
        <p className="text-sm font-medium">{t("workbench.rehearsal.noCandidate")}</p>
        <p className="text-sm text-muted-foreground">{t("workbench.rehearsal.noCandidateBody")}</p>
      </div>
    );
  }
  return <RehearsalForm agentId={agentId} candidates={candidates.data} onOpened={onOpened} />;
}

function RehearsalForm({ agentId, candidates, onOpened }: { agentId: string; candidates: CandidateSeal[]; onOpened: (runId: string) => void }) {
  const { t } = useTranslation();
  const open = useOpenRehearsal(agentId);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { sealId: candidates[0]?.sealId ?? "", text: "" } });

  const submit = (values: Values) =>
    open.mutate(
      { sealId: values.sealId, case: values.text },
      { onSuccess: (run) => onOpened(run.runId), onError: (error) => toast.error(problemMessage(error, t)) },
    );

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} className="flex flex-col gap-4">
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
          name="text"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("workbench.rehearsal.case")}</FormLabel>
              <FormControl><Textarea {...field} rows={5} placeholder={t("workbench.rehearsal.casePlaceholder")} /></FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type="submit" variant="outline" className="self-start" disabled={open.isPending}>
          <FlaskConical className="size-4" aria-hidden />
          {t("workbench.rehearsal.open")}
        </Button>
      </form>
    </Form>
  );
}
