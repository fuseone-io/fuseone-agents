import { useTranslation } from "react-i18next";
import { Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { PAGE_ICONS } from "@/components/layout/nav";
import { useAgent } from "@/features/agents/agent-detail-api";
import { ShieldCheck } from "lucide-react";
import { AuthoringToday } from "@/features/workbench/authoring-today";
import { WorkbenchConversation } from "@/features/workbench/workbench-conversation";
import { WorkbenchDiscard } from "@/features/workbench/workbench-discard";
import { WorkbenchFields } from "@/features/workbench/workbench-fields";
import { WorkbenchInstructions } from "@/features/workbench/workbench-instructions";
import { WorkbenchMissing } from "@/features/workbench/workbench-missing";
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
  // The published version, when there is one, is what the diff is against.
  const published = useAgent(draft.body.baseVersionId ? agentId : "", undefined);
  const count = bench.pending.length;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        icon={PAGE_ICONS.agents}
        title={agentId}
        description={t("workbench.description", { agent: bench.local.name || agentId })}
      >
        <AuthoringToday agentId={agentId} />
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
      {/* The conversation on the left and the workspace on the right, as in
          NT-018's mockup; on a narrow screen the workspace wraps below it. */}
      <div className="flex flex-wrap items-start gap-6">
        <div className="min-w-0 flex-[1_1_400px]">
          <WorkbenchConversation
            agentId={agentId}
            instructions={draft.body.definition.instructions}
            onDraft={bench.adopt}
          />
        </div>
        <div className="flex min-w-0 flex-[999_1_600px] flex-col gap-6">
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <ShieldCheck className="size-4" aria-hidden />
            {t("workbench.untouchedProduction")}
          </p>
          <WorkbenchMissing missing={draft.body.missing} />
          <WorkbenchFields agentId={agentId} local={bench.local} patch={bench.patch} />
          <WorkbenchInstructions
            now={draft.body.definition.instructions}
            published={published.data?.instructions}
          />
        </div>
      </div>
    </div>
  );
}
