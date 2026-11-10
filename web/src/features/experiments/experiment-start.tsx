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
import { ExperimentConfigFields } from "@/features/experiments/experiment-config-fields";
import { ExperimentCost } from "@/features/experiments/experiment-cost";
import { RepetitionsField, VersionField } from "@/features/experiments/experiment-pin-fields";
import {
  experimentDefaults,
  experimentForm,
  experimentRequestOf,
  type ExperimentForm,
} from "@/features/experiments/experiment-form";
import { useStartExperiment } from "@/features/experiments/experiments-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Starting an experiment: one agent version, against the latest dataset
 * version, under a configuration, once or more per case.
 *
 * It never changes the agent. Moving to the model that won is publishing a new
 * version, with its own battery.
 */
export function ExperimentStart({
  agentId,
  onClose,
  onStarted,
}: {
  agentId: string;
  onClose: () => void;
  onStarted: (id: string) => void;
}) {
  const { t } = useTranslation();
  const detail = useAgent(agentId);
  const start = useStartExperiment(agentId);
  const versions = detail.data?.versions ?? [];

  const form = useForm<ExperimentForm>({
    resolver: zodResolver(experimentForm),
    values: experimentDefaults(detail.data?.agent.versionId ?? ""),
    resetOptions: { keepDirtyValues: true },
  });

  const repetitions = useWatch({ control: form.control, name: "repetitions" });
  // The version chosen, read as itself: its provider is what the models are offered for and
  // its ceiling is what the cost is stated at, since that is the version whose runs open.
  const version = useWatch({ control: form.control, name: "version" });
  const agent = useAgent(agentId, version || undefined).data?.agent;

  async function submit(values: ExperimentForm) {
    try {
      const started = await start.mutateAsync(experimentRequestOf(values));
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
            <VersionField form={form} versions={versions} />
            <ExperimentConfigFields form={form} publishedProvider={agent?.provider ?? ""} />
            <RepetitionsField form={form} />
            <ExperimentCost
              agentId={agentId}
              agent={agent}
              repetitions={Number(repetitions)}
            />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={start.isPending || !agent}>
              {t("experiments.start")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
