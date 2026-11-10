import { useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Plus, TestTubes } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { useAgent } from "@/features/agents/agent-detail-api";
import { ExperimentComparisonView } from "@/features/experiments/experiment-comparison";
import { ExperimentList } from "@/features/experiments/experiment-list";
import { ExperimentReportView } from "@/features/experiments/experiment-report";
import { ExperimentStart } from "@/features/experiments/experiment-start";
import { useExperiment } from "@/features/experiments/experiments-api";

/**
 * An agent's experiments: one version against a dataset version, under another
 * configuration (NT-015 § Experiments).
 *
 * What is on screen travels in the URL — a report, a comparison, the form — so a
 * refresh keeps it and a link to it reaches the same place. Everything shown is
 * a fold of runs, never state this screen holds.
 */
export function ExperimentsPage() {
  const { t } = useTranslation();
  const { agentId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const agent = useAgent(agentId);
  const id = params.get("exp") ?? "";
  const from = params.get("from") ?? "";
  const to = params.get("to") ?? "";
  const starting = params.get("new") === "1";
  const report = useExperiment(agentId, id);
  const comparing = from !== "" && to !== "";

  return (
    <div className="flex w-full min-w-0 flex-col gap-5">
      <PageHeader
        icon={TestTubes}
        title={t("experiments.title")}
        description={t("experiments.subtitle", { agent: agent.data?.agent.name || agentId })}
      >
        {(id !== "" || comparing) && (
          <Button variant="outline" onClick={() => setParams({})}>
            {t("experiments.all")}
          </Button>
        )}
        <Button onClick={() => setParams({ ...Object.fromEntries(params), new: "1" })}>
          <Plus className="size-4" aria-hidden />
          {t("experiments.new")}
        </Button>
      </PageHeader>

      {id !== "" && <ExperimentReportView report={report} />}
      {id === "" && comparing && <ExperimentComparisonView agentId={agentId} from={from} to={to} />}
      {id === "" && !comparing && (
        <ExperimentList
          agentId={agentId}
          onOpen={(exp) => setParams({ exp })}
          onCompare={(f, t2) => setParams({ from: f, to: t2 })}
        />
      )}

      {starting && (
        <ExperimentStart
          agentId={agentId}
          onClose={() => {
            params.delete("new");
            setParams(params);
          }}
          onStarted={(exp) => setParams({ exp }, { replace: true })}
        />
      )}
    </div>
  );
}
