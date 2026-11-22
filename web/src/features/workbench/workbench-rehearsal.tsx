import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { RehearsalRun } from "@/features/workbench/rehearsal-run";
import { RehearsalStart } from "@/features/workbench/rehearsal-start";

/**
 * The rehearsal tab (NT-018 phase 4, release 4c): a candidate run on one case the author
 * writes, reaching no tool. The rehearsal being followed is named in the address, so it
 * survives switching tabs, reloading, and being shared with another author of the area.
 */
export function WorkbenchRehearsal({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const runId = params.get("rehearsal") ?? "";

  const follow = (id: string) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (id) next.set("rehearsal", id);
      else next.delete("rehearsal");
      return next;
    });

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
