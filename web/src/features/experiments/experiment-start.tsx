import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, useWatch } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { useAgent } from "@/features/agents/agent-detail-api";
import { agentTarget } from "@/features/agents/agent-target";
import { ExperimentConfigFields } from "@/features/experiments/experiment-config-fields";
import { ExperimentCost } from "@/features/experiments/experiment-cost";
import { CandidateLine, RepetitionsField, VersionField } from "@/features/experiments/experiment-pin-fields";
import {
  experimentDefaults,
  experimentForm,
  experimentRequestOf,
  type ExperimentForm,
} from "@/features/experiments/experiment-form";
import { useStartExperiment } from "@/features/experiments/experiments-api";
import { useSeal } from "@/features/workbench/candidates-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Starting an experiment: one agent version, against the latest dataset
 * version, under a configuration, once or more per case.
 *
 * It never changes the agent. Moving to the model that won is publishing a new
 * version, with its own battery. With a seal it runs that candidate instead
 * (NT-018 phase 2), which needs no published version of the agent at all.
 */
export function ExperimentStart({
  agentId,
  sealId,
  onClose,
  onStarted,
}: {
  agentId: string;
  /** A candidate's seal to run instead of a published version. */
  sealId?: string;
  onClose: () => void;
  onStarted: (id: string) => void;
}) {
  const { t } = useTranslation();
  const detail = useAgent(agentId);
  // The seal alone decides what runs, so the candidate named is the seal's own, read by
  // its id; nothing else in the URL is believed.
  const sealRead = useSeal(sealId ? agentId : "", sealId ?? "");
  const seal = sealRead.data ?? undefined;
  const unresolved = sealId !== undefined && seal === undefined;
  const start = useStartExperiment(agentId);
  const versions = detail.data?.versions ?? [];

  const form = useForm<ExperimentForm>({
    resolver: zodResolver(experimentForm),
    // A candidate's version only fills the form; the request names the seal.
    values: experimentDefaults(sealId ? (seal?.version ?? "") : (detail.data?.agent.versionId ?? "")),
    resetOptions: { keepDirtyValues: true },
  });

  const repetitions = useWatch({ control: form.control, name: "repetitions" });
  // The version chosen, read as itself: its provider is what the models are offered for and
  // its ceiling is what the cost is stated at, since that is the version whose runs open.
  const version = useWatch({ control: form.control, name: "version" });
  // A candidate is no published version to read: the published agent, if any, is
  // what its models are offered for and its cost is stated at.
  const agent = useAgent(agentId, sealId ? undefined : version || undefined).data?.agent;

  async function submit(values: ExperimentForm) {
    try {
      const started = await start.mutateAsync(experimentRequestOf(values, sealId));
      toast.success(t("experiments.started", { count: started.runs }));
      onStarted(started.id);
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("experiments.startTitle")}
      description={t("experiments.startExplains")}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(submit)} className="flex min-h-0 flex-1 flex-col">
          <PropertiesSheetBody className="space-y-4">
            {sealId ? (
              <CandidateLine seal={sealRead} />
            ) : (
              <VersionField form={form} versions={versions} />
            )}
            <ExperimentConfigFields
              form={form}
              publishedProvider={agent?.provider ?? ""}
              target={agent ? agentTarget(agent.scope) : undefined}
            />
            <RepetitionsField form={form} />
            <ExperimentCost
              agentId={agentId}
              agent={agent}
              repetitions={Number(repetitions)}
              candidate={sealId !== undefined}
            />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={start.isPending || (sealId ? unresolved : !agent)}>
              {t("experiments.start")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
