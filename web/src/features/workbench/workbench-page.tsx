import { useParams } from "react-router-dom";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { isAbsent, useDraft } from "@/features/workbench/api";
import { WorkbenchNoDraft } from "@/features/workbench/workbench-no-draft";
import { WorkbenchPanel } from "@/features/workbench/workbench-panel";

/**
 * /agents/:agentId/workbench: the agent's draft, or the way to open one.
 *
 * A draft once shown stays shown: a later read that fails, or finds the draft
 * gone, keeps the panel and its unsaved edits on screen, and the panel says
 * what happened. Only a first read decides between the panel and its absence.
 */
export function WorkbenchPage() {
  const { agentId = "" } = useParams();
  const draft = useDraft(agentId);

  if (draft.data) {
    return (
      <WorkbenchPanel
        agentId={agentId}
        draft={draft.data}
        reread={async () => {
          const next = await draft.refetch();
          return next.isError ? undefined : next.data;
        }}
      />
    );
  }
  if (draft.isLoading) return <LoadingRows rows={6} />;
  if (isAbsent(draft.error)) return <WorkbenchNoDraft agentId={agentId} />;
  return <ErrorState error={draft.error} onRetry={() => void draft.refetch()} />;
}
