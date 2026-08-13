import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmAction } from "@/features/runs/confirm-action";
import { useDecideApproval } from "@/features/runs/api";
import type { PendingApproval } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Approve or refuse, pinned to the bottom of the trace.
 *
 * atSeq travels with the decision, exactly as it does on the run's own screen:
 * a panel left open while the run moved on must not answer whatever it is
 * waiting for now. The server refuses the mismatch rather than applying it to
 * a different action.
 */
export function TraceActions({
  runId,
  approval,
}: {
  runId: string;
  approval: PendingApproval;
}) {
  const { t } = useTranslation();
  const decide = useDecideApproval(runId);

  const submit = (approved: boolean) =>
    decide.mutate(
      { approved, atSeq: approval.atSeq },
      {
        onSuccess: () =>
          toast.success(approved ? t("runs.actionApproved") : "approvals.actionRefused"),
        onError: (error) =>
          toast.error(t("runs.decisionFailed"), {
            description: problemMessage(error, t),
          }),
      },
    );

  return (
    <div className="flex gap-2">
      <ConfirmAction
        label={t("runs.approve")}
        title={t("runs.approveThis")}
        description={t("approvals.willRunNamed", { tool: approval.tool })}
        disabled={decide.isPending}
        onConfirm={() => submit(true)}
      />
      <Button
        variant="outline"
        className="h-[34px] flex-1"
        disabled={decide.isPending}
        onClick={() => submit(false)}
      >
        {t("runs.refuse")}
      </Button>
    </div>
  );
}
