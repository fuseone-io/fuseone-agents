import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ShieldCheck } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAgent } from "@/features/agents/agent-detail-api";
import { WorkbenchDraftSummary } from "@/features/workbench/workbench-draft-summary";
import { WorkbenchEditSheet } from "@/features/workbench/workbench-edit-sheet";
import { WorkbenchPane } from "@/features/workbench/workbench-pane";
import type { FieldPart } from "@/features/workbench/workbench-field-part";
import type { AgentDefinition, AgentDraft } from "@/lib/api/client";

/**
 * The workspace, in tabs as NT-018's mockup draws it. Today it has one, the
 * draft; the rehearsal, experiments and cases join it with their phases, and
 * no tab is shown before there is something behind it.
 */
export function WorkbenchWorkspace({
  agentId,
  draft,
  local,
  patch,
}: {
  agentId: string;
  draft: AgentDraft;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const [part, setPart] = useState<FieldPart>();
  // The published version, when there is one, is what the instructions are read against.
  const published = useAgent(draft.baseVersionId ? agentId : "", undefined);

  return (
    <Tabs defaultValue="draft" className="h-full">
      <WorkbenchPane
        label={t("workbench.workspace")}
        header={
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2">
            <TabsList>
              <TabsTrigger value="draft">{t("workbench.tabs.draft")}</TabsTrigger>
            </TabsList>
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
      </WorkbenchPane>
      <WorkbenchEditSheet part={part} onClose={() => setPart(undefined)} editing={{ agentId, local, patch }} />
    </Tabs>
  );
}
