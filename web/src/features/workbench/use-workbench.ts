import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { operationsFor } from "@/features/workbench/draft-operations";
import { useDraftEdits } from "@/features/workbench/use-draft-edits";
import { useSaveDraft } from "@/features/workbench/use-save-draft";
import type { RevisedDraft } from "@/features/workbench/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The panel over one draft: what the person changed, what saving would send,
 * and the two refusals a save can meet. Publication is not here.
 */
/** Reads the draft again: the newer one, or nothing when it is still not there. */
export type Reread = () => Promise<RevisedDraft | undefined>;

export function useWorkbench(agentId: string, draft: RevisedDraft, reread: Reread) {
  const { t } = useTranslation();
  const edits = useDraftEdits(draft.body.definition);
  const saving = useSaveDraft(agentId);
  const local = edits.local ?? draft.body.definition;
  const pending = operationsFor(draft.body.definition, local, edits.touched);

  const save = () => {
    // What this save was computed from: answered, these values are the
    // server's, and anything typed after this moment stays unsaved.
    const sent = local;
    void saving.save(draft, pending, {
      draft: (next, last) => {
        edits.takeIn(next.body.definition, last ? sent : undefined);
        if (last) toast.success(t("workbench.saved"));
      },
      error: (error) => toast.error(problemMessage(error, t)),
    });
  };

  /**
   * Reads the draft again and keeps every unsaved edit over it. A draft still
   * not there leaves the panel, and the notice, as they were.
   */
  const reload = async () => {
    const next = await reread();
    if (!next) return;
    edits.takeIn(next.body.definition);
    saving.settle();
  };

  return { local, patch: edits.patch, pending, save, reload, state: saving.state, landed: saving.landed };
}
