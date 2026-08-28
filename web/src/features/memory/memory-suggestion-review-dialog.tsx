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
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  useAcceptMemorySuggestion,
  useDismissMemorySuggestion,
  type MemorySuggestion,
} from "@/features/memory/api";
import { problemMessage } from "@/lib/api/problem-message";

export function MemorySuggestionReviewDialog({
  suggestion,
  action,
  onClose,
}: {
  suggestion: MemorySuggestion;
  action: "accept" | "dismiss";
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const accept = useAcceptMemorySuggestion();
  const dismiss = useDismissMemorySuggestion();
  const mutation = action === "accept" ? accept : dismiss;
  const [reason, setReason] = useState("");
  /*
    The claim starts as the agent proposed it, and only a change is sent.

    Sending it back unchanged would record an agreement to words somebody
    retyped rather than to the words that were proposed, and the two are
    different facts about the review. Only the claim is editable: subject,
    signature and kind are the identity, and changing one here would mark this
    proposal accepted for a fact nobody proposed.
  */
  const [claim, setClaim] = useState(suggestion.claim);
  const rewritten = claim.trim() !== suggestion.claim.trim();

  const submit = () =>
    mutation.mutate(
      {
        id: suggestion.id,
        company: suggestion.scope.company,
        area: suggestion.scope.area,
        reason: reason.trim(),
        ...(action === "accept" && rewritten ? { claim: claim.trim() } : {}),
      },
      {
        onSuccess: () => {
          toast.success(t(action === "accept" ? "memory.accepted" : "memory.dismissed"));
          onClose();
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(action === "accept" ? "memory.acceptTitle" : "memory.dismissTitle")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(action === "accept" ? "memory.acceptHint" : "memory.dismissHint")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {action === "accept" && (
          <div className="grid gap-1.5">
            <Label htmlFor="memory-suggestion-claim">{t("memory.claim")}</Label>
            <Textarea
              id="memory-suggestion-claim"
              className="min-h-24"
              value={claim}
              onChange={(event) => setClaim(event.target.value)}
            />
            <p className="text-2xs text-muted-foreground">
              {t("memory.acceptClaimHint")}
            </p>
          </div>
        )}
        <div className="grid gap-1.5">
          <Label htmlFor="memory-suggestion-reason">{t("memory.reason")}</Label>
          <Input
            id="memory-suggestion-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t("memory.reviewReasonPlaceholder")}
          />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant={action === "dismiss" ? "destructive" : "default"}
            disabled={
              reason.trim() === "" ||
              (action === "accept" && claim.trim() === "") ||
              mutation.isPending
            }
            onClick={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            {t(action === "accept" ? "memory.acceptSuggestion" : "memory.dismissSuggestion")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
