import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import {
  useCreateMemoryAssertion,
  useMemoryMatch,
} from "@/features/memory/api";
import { MemoryDuplicateNotice } from "@/features/memory/memory-duplicate-notice";
import { MemoryCitationField } from "@/features/memory/memory-citation-field";
import { MemoryEvidenceChip } from "@/features/memory/memory-evidence-chip";
import { MemoryFactFields } from "@/features/memory/memory-fact-fields";
import { MemoryInputField } from "@/features/memory/memory-form-fields";
import { MemoryNamespaceField } from "@/features/memory/memory-namespace-field";
import {
  memoryFromRun,
  toMemoryAssertionInput,
} from "@/features/memory/memory-create-model";
import {
  memoryFormSchema,
  type MemoryFormValues,
} from "@/features/memory/memory-form-schema";
import { useRunSteps } from "@/features/runs/api";
import { citationsOf, labelsUpTo } from "@/features/runs/run-citations";
import { problemMessage } from "@/lib/api/problem-message";
import type { Run, Step } from "@/lib/api/client";

/**
 * Teaching a memory from the run in front of you.
 *
 * Everything the platform can answer is answered: the scope and the run come
 * from the record being read, the citation from the step, the labels from the
 * trail, the agent from the ledger once the request arrives. What is asked for
 * is what only a person knows — what kind of fact this is, what it is about,
 * what it claims, who reads it, and why they are recording it.
 */
export function RememberThisForm({
  runId,
  step,
  run,
  onDone,
}: {
  runId: string;
  step: Step;
  run: Pick<Run, "scope" | "agentId">;
  onDone: () => void;
}) {
  const { scope, agentId } = run;
  const { t } = useTranslation();
  const create = useCreateMemoryAssertion();
  const citations = citationsOf(step);
  const first = citations[0];
  const form = useForm<MemoryFormValues>({
    resolver: zodResolver(memoryFormSchema),
    mode: "onChange",
    defaultValues: memoryFromRun(scope, runId, first?.artifact ?? ""),
  });
  // The trail the page already holds. Same query key, so this reads the cache
  // rather than asking for a run the screen behind it is displaying.
  const trail = useRunSteps(runId);
  const chosen = form.watch("evidenceArtifact");
  // Asked about what is being typed, so the answer is there before the decision
  // rather than as a conflict afterwards. The agent is the run's own: unlike
  // creation there is no evidence to read it from yet, because nothing has been
  // composed, and the run in front of the person is whose namespace this is.
  const namespace = form.watch("namespace");
  const match = useMemoryMatch({
    company: scope.company,
    area: scope.area,
    namespace,
    ...(namespace === "agent" && agentId ? { agentId } : {}),
    kind: form.watch("kind"),
    subject: form.watch("subject"),
    signature: form.watch("signature"),
  });

  // The button is only offered where there is something to cite, so this is an
  // impossible state rather than an empty one — and a form with no evidence
  // would be a form that cannot be submitted, with nothing saying why.
  if (!first) return null;

  const citation = citations.find((c) => c.artifact === chosen) ?? first;

  async function submit(values: MemoryFormValues) {
    try {
      await create.mutateAsync(toMemoryAssertionInput(values));
      toast.success(t("memory.recorded"));
      onDone();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} className="grid gap-4">
        <MemoryCitationField control={form.control} citations={citations} />
        <MemoryEvidenceChip
          runId={runId}
          citation={citation}
          labels={labelsUpTo(trail.items, step.seq)}
        />
        <MemoryNamespaceField control={form.control} />
        <MemoryDuplicateNotice
          match={match.data}
          reason={form.watch("reason")}
          onImproveShared={() => form.setValue("namespace", "shared")}
        />
        <MemoryFactFields control={form.control} />
        <MemoryInputField
          control={form.control}
          name="reason"
          label="memory.reason"
          placeholder="memory.reasonPlaceholder"
        />
        <p className="text-2xs text-muted-foreground">{t("memory.ttlNote")}</p>
        <Button
          type="submit"
          disabled={!form.formState.isValid || create.isPending}
          className="justify-self-start"
        >
          {t("memory.saveMemory")}
        </Button>
      </form>
    </Form>
  );
}
