import { useTranslation } from "react-i18next";
import { Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { PAGE_ICONS } from "@/components/layout/nav";
import { AuthoringToday } from "@/features/workbench/authoring-today";
import { WorkbenchConversation } from "@/features/workbench/workbench-conversation";
import { WorkbenchDiscard } from "@/features/workbench/workbench-discard";
import { WorkbenchPanes } from "@/features/workbench/workbench-panes";
import { WorkbenchWorkspace } from "@/features/workbench/workbench-workspace";
import { WorkbenchNotice } from "@/features/workbench/workbench-notice";
import { useWorkbench, type Reread } from "@/features/workbench/use-workbench";
import type { RevisedDraft } from "@/features/workbench/api";

/**
 * One agent's draft, worked on in the panel (NT-018 1b).
 *
 * Every change goes to the draft, under its ETag, and nothing here publishes:
 * the agent in production stays as it is. The one action is saving; the
 * instructions are read against the published version and change only by a
 * proposal of the conversation its author accepts.
 */
export function WorkbenchPanel({
  agentId,
  draft,
  reread,
}: {
  agentId: string;
  draft: RevisedDraft;
  reread: Reread;
}) {
  const { t } = useTranslation();
  const bench = useWorkbench(agentId, draft, reread);
  const count = bench.pending.length;

  return (
    // As tall as the screen allows: the panes scroll, the page does not.
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <PageHeader
        icon={PAGE_ICONS.agents}
        title={agentId}
        description={t("workbench.description", { agent: bench.local.name || agentId })}
      >
        <AuthoringToday agentId={agentId} className="hidden sm:inline" />
        <WorkbenchDiscard agentId={agentId} etag={draft.etag} />
        <Button size="sm" onClick={bench.save} disabled={count === 0 || bench.state === "saving"}>
          <Save className="size-4" aria-hidden />
          {/* The icon alone on a phone, where the header holds two actions
              beside the breadcrumb; the words stay the button's name. */}
          <span className="sr-only sm:not-sr-only">
            {count === 0
              ? t("workbench.nothingToSave")
              : `${t("workbench.save")} · ${t("workbench.saveCount", { count })}`}
          </span>
        </Button>
      </PageHeader>
      <WorkbenchNotice state={bench.state} landed={bench.landed} onReread={() => void bench.reload()} />
      <WorkbenchPanes
        conversation={
          <WorkbenchConversation agentId={agentId} instructions={draft.body.definition.instructions} onDraft={bench.adopt} />
        }
        workspace={<WorkbenchWorkspace agentId={agentId} draft={draft.body} local={bench.local} patch={bench.patch} />}
      />
    </div>
  );
}
