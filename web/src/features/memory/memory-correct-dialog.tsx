import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  useCreateMemoryAssertion,
  type MemoryAssertion,
  type MemoryAssertionInput,
} from "@/features/memory/api";
import { problemMessage } from "@/lib/api/problem-message";

export function MemoryCorrectDialog({
  assertion,
  onClose,
}: {
  assertion: MemoryAssertion;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const correct = useCreateMemoryAssertion();
  const [claim, setClaim] = useState(assertion.claim);
  const [reason, setReason] = useState("");
  const cleanClaim = claim.trim();
  const cleanReason = reason.trim();

  async function submit() {
    try {
      await correct.mutateAsync(correctionInput(assertion, cleanClaim, cleanReason));
      toast.success(t("memory.corrected"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("memory.correctTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("memory.correctHint")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="memory-correct-claim">{t("memory.claim")}</Label>
            <Textarea
              id="memory-correct-claim"
              className="min-h-28"
              value={claim}
              onChange={(event) => setClaim(event.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="memory-correct-reason">{t("memory.reason")}</Label>
            <Input
              id="memory-correct-reason"
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder={t("memory.correctReasonPlaceholder")}
            />
          </div>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            disabled={!canSubmit(assertion.claim, cleanClaim, cleanReason) || correct.isPending}
            onClick={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            {t("memory.correct")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function canSubmit(original: string, claim: string, reason: string): boolean {
  return claim !== "" && claim !== original.trim() && reason !== "";
}

function correctionInput(
  assertion: MemoryAssertion,
  claim: string,
  reason: string,
): MemoryAssertionInput {
  return {
    company: assertion.scope.company,
    area: assertion.scope.area,
    agentId: assertion.agentId === "" ? undefined : assertion.agentId,
    kind: assertion.kind,
    subject: assertion.subject,
    signature: assertion.signature,
    claim,
    observations: assertion.observations,
    confirmed: assertion.confirmed,
    evidence: assertion.evidence,
    expiresAt: assertion.expiresAt ?? undefined,
    reason,
  };
}
