import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { RehearsalRun } from "@/features/workbench/rehearsal-run";
import { RehearsalStart } from "@/features/workbench/rehearsal-start";
import { RehearsalTrace } from "@/features/workbench/rehearsal-trace";

/**
 * The rehearsal tab (NT-018 phase 4, release 4c): a candidate run on one case the author
 * writes, reaching no tool. The rehearsal being followed is named in the address, so it
 * survives switching tabs, reloading, and being shared with another author of the area.
 *
 * A trace named beside it wins: it is a run opened to be read (phase 5c), and showing the
 * rehearsal instead would offer actions on the run the person did not ask about.
 */
export function WorkbenchRehearsal({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const runId = params.get("rehearsal") ?? "";
  const trace = params.get("trace") ?? "";

  const set = (name: "rehearsal" | "trace", id: string) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (id) next.set(name, id);
      else next.delete(name);
      return next;
    });
  const follow = (id: string) => set("rehearsal", id);

  if (trace) return <RehearsalTrace runId={trace} onClose={() => set("trace", "")} />;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex max-w-prose flex-col gap-1">
        <p className="text-sm text-muted-foreground">{t("workbench.rehearsal.intro")}</p>
        <p className="text-xs text-muted-foreground">{t("workbench.rehearsal.erasureNote")}</p>
      </div>
      {runId ? <RehearsalRun agentId={agentId} runId={runId} onNew={() => follow("")} /> : <RehearsalStart agentId={agentId} onOpened={follow} />}
    </div>
  );
}
