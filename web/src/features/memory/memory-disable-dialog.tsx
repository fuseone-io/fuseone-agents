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
import { useDisableMemoryAssertion, type MemoryAssertion } from "@/features/memory/api";
import { problemMessage } from "@/lib/api/problem-message";

export function MemoryDisableDialog({
  assertion,
  onClose,
}: {
  assertion: MemoryAssertion;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const disable = useDisableMemoryAssertion();
  const [reason, setReason] = useState("");

  const submit = () =>
    disable.mutate(
      {
        id: assertion.id,
        company: assertion.scope.company,
        area: assertion.scope.area,
        reason: reason.trim(),
      },
      {
        onSuccess: () => {
          toast.success(t("memory.disabled"));
          onClose();
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("memory.disableTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("memory.disableHint")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="memory-disable-reason">{t("memory.reason")}</Label>
          <Input
            id="memory-disable-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t("memory.disableReasonPlaceholder")}
          />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={reason.trim() === "" || disable.isPending}
            onClick={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            {t("memory.disable")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
