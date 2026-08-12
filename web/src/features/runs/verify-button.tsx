import { useTranslation } from "react-i18next";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useVerifyRun } from "@/features/runs/api";

/**
 * Verification walks every step, so it runs on request rather than on page
 * load. The result is stated in words, never only in colour — and a broken
 * chain names the step it broke at, because "invalid" alone tells an auditor
 * nothing they can act on.
 */
export function VerifyButton({ runId }: { runId: string }) {
  const verify = useVerifyRun(runId);
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        onClick={() => verify.mutate()}
        disabled={verify.isPending}
      >
        <ShieldCheck className="size-4" />
        {t("runs.verifyTrail")}
      </Button>
      {verify.data && (
        <span
          role="status"
          className={
            verify.data.valid ? "text-sm text-success" : "text-sm text-danger"
          }
        >
          {verify.data.valid
            ? t("runs.verifyIntact", { count: verify.data.stepsChecked })
            : t("runs.verifyBroken", { seq: verify.data.brokenAtSeq })}
        </span>
      )}
    </div>
  );
}
