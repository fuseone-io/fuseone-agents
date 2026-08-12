import { useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { FlaskConical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { useSimulation } from "@/features/agents/simulation-api";
import { SimulationStart } from "@/features/agents/simulation-start";
import { SimulationReportView } from "@/features/agents/simulation-report";

/**
 * An agent, run against occurrences that already happened.
 *
 * The central safety mechanism, and the only validation legible to somebody
 * who cannot read a specification: a description of a process is always
 * incomplete, because people describe the happy path and omit the exception.
 *
 * The simulation travels in the URL so a refresh keeps it and a link to it
 * reaches the same report — the report is a fold of the runs, not a page of
 * state this screen is holding.
 */
export function SimulationPage() {
  const { t } = useTranslation();
  const { agentId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const id = params.get("sim") ?? "";

  const report = useSimulation(agentId, id);

  return (
    <div className="flex w-full min-w-0 flex-col gap-5">
      <PageHeader
        icon={FlaskConical}
        title={t("simulation.title")}
        description={agentId}
      >
        {id !== "" && (
          <Button variant="outline" onClick={() => setParams({})}>
            {t("simulation.again")}
          </Button>
        )}
      </PageHeader>

      {id === "" ? (
        <SimulationStart
          agentId={agentId}
          onStarted={(started) =>
            setParams({ sim: started }, { replace: true })
          }
        />
      ) : (
        <SimulationReportView agentId={agentId} report={report} />
      )}
    </div>
  );
}
