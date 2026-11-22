import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { ShieldCheck } from "lucide-react";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAgent } from "@/features/agents/agent-detail-api";
import { WorkbenchCandidates, type Sealing } from "@/features/workbench/workbench-candidates";
import { WorkbenchCases } from "@/features/workbench/workbench-cases";
import { WorkbenchDraftSummary } from "@/features/workbench/workbench-draft-summary";
import { WorkbenchExperiment } from "@/features/workbench/workbench-experiment";
import { WorkbenchEditSheet } from "@/features/workbench/workbench-edit-sheet";
import { WorkbenchPane } from "@/features/workbench/workbench-pane";
import { WorkbenchRehearsal } from "@/features/workbench/workbench-rehearsal";
import type { FieldPart } from "@/features/workbench/workbench-field-part";
import type { AgentDefinition, AgentDraft } from "@/lib/api/client";

/**
 * The workspace, in tabs as NT-018's mockup draws it: the draft, the candidates
 * sealed from it (phase 2), the rehearsal of one (phase 4), the cases kept
 * from rehearsals (phase 5), and a candidate run against them (phase 5c).
 */
export function WorkbenchWorkspace({
  agentId,
  draft,
  local,
  patch,
  sealing,
}: {
  agentId: string;
  draft: AgentDraft;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  sealing: Sealing;
}) {
  const { t } = useTranslation();
  const [part, setPart] = useState<FieldPart>();
  // The published version, when there is one, is what the instructions are read against.
  const published = useAgent(draft.baseVersionId ? agentId : "", undefined);
  // What the address names opens on its tab: the address was shared to show it.
  const [params, setParams] = useSearchParams();
  const [tab, setTab] = useState(() => openingTab(params));
  // A regression's run is read in the rehearsal tab, beside the experiment it came from.
  const trace = (runId: string) => {
    setParams((current) => {
      const next = new URLSearchParams(current);
      next.set("trace", runId);
      return next;
    });
    setTab("rehearsal");
  };

  return (
    <Tabs value={tab} onValueChange={setTab} className="h-full">
      <WorkbenchPane
        label={t("workbench.workspace")}
        header={
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2">
            {/* Five tabs outgrow a phone: the strip scrolls rather than cutting the last one off. */}
            <ScrollArea className="min-w-0 max-w-full">
              <TabsList>
                <TabsTrigger value="draft" className="flex-none">{t("workbench.tabs.draft")}</TabsTrigger>
                <TabsTrigger value="candidates" className="flex-none">{t("workbench.tabs.candidates")}</TabsTrigger>
                <TabsTrigger value="rehearsal" className="flex-none">{t("workbench.tabs.rehearsal")}</TabsTrigger>
                <TabsTrigger value="cases" className="flex-none">{t("workbench.tabs.cases")}</TabsTrigger>
                <TabsTrigger value="experiment" className="flex-none">{t("workbench.tabs.experiment")}</TabsTrigger>
              </TabsList>
              <ScrollBar orientation="horizontal" />
            </ScrollArea>
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <ShieldCheck className="size-3.5" aria-hidden />
              {t("workbench.untouchedProduction")}
            </span>
          </div>
        }
      >
        <TabsContent value="draft">
          <WorkbenchDraftSummary draft={draft} local={local} published={published.data?.instructions} onEdit={setPart} patch={patch} />
        </TabsContent>
        <TabsContent value="candidates" className="p-4">
          <WorkbenchCandidates agentId={agentId} sealing={sealing} />
        </TabsContent>
        <TabsContent value="rehearsal" className="p-4">
          <WorkbenchRehearsal agentId={agentId} />
        </TabsContent>
        <TabsContent value="cases" className="p-4">
          <WorkbenchCases agentId={agentId} />
        </TabsContent>
        <TabsContent value="experiment" className="p-4">
          <WorkbenchExperiment agentId={agentId} onTrace={trace} />
        </TabsContent>
      </WorkbenchPane>
      <WorkbenchEditSheet part={part} onClose={() => setPart(undefined)} editing={{ agentId, local, patch }} />
    </Tabs>
  );
}

function openingTab(params: URLSearchParams): string {
  if (params.has("trace") || params.has("rehearsal")) return "rehearsal";
  return params.has("experiment") ? "experiment" : "draft";
}
