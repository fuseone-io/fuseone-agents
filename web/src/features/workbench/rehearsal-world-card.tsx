import { useId } from "react";
import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Globe } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { StepContent } from "@/features/runs/step-content";
import { useAnswerWorld } from "@/features/workbench/rehearsal-api";
import { fitsIn } from "@/features/workbench/rehearsal-text";
import { problemMessage } from "@/lib/api/problem-message";

/** The server's bound on one answer for the world. */
const MAX_ANSWER_BYTES = 256 * 1024;

const schema = z.object({
  answer: z
    .string()
    .min(1, "workbench.rehearsal.answerRequired")
    .refine(fitsIn(MAX_ANSWER_BYTES), "workbench.rehearsal.answerTooLong"),
});
type Values = z.infer<typeof schema>;

/** The wait a rehearsal stands at: the call it made, by tool and step. */
export type WorldWait = { atSeq: number; tool: string; callSeq: number };

/**
 * The call the rehearsal waits on, and the author's answer for it. The call reached no
 * tool: what is written here is what the agent reads back as the tool's result.
 */
export function RehearsalWorldCard({ runId, wait }: { runId: string; wait: WorldWait }) {
  const { t } = useTranslation();
  const titleId = useId();
  const answer = useAnswerWorld(runId);
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { answer: "" } });

  const submit = (values: Values) =>
    answer.mutate({ atSeq: wait.atSeq, answer: values.answer }, { onError: (error) => toast.error(problemMessage(error, t)) });

  return (
    <Card role="region" aria-labelledby={titleId}>
      <CardHeader>
        <CardTitle id={titleId} className="flex items-center gap-2 text-sm">
          <Globe className="size-4" aria-hidden />
          {t("workbench.rehearsal.worldTitle")}
        </CardTitle>
        <CardDescription>{t("workbench.rehearsal.worldBody")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div>
          <p className="text-xs text-muted-foreground">{t("workbench.rehearsal.tool")}</p>
          <Mono>{wait.tool}</Mono>
          <p className="mt-3 text-xs text-muted-foreground">{t("workbench.rehearsal.arguments")}</p>
          <StepContent runId={runId} seq={wait.callSeq} open />
        </div>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} className="flex flex-col gap-3">
            <FormField
              control={form.control}
              name="answer"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("workbench.rehearsal.answer")}</FormLabel>
                  <FormControl><Textarea {...field} rows={6} className="font-mono" /></FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button type="submit" variant="outline" className="self-start" disabled={answer.isPending}>
              {t("workbench.rehearsal.answerSend")}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
