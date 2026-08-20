import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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
import { useEraseContent } from "@/features/admin/retention-api";

/**
 * Erasing what a set of runs was about, on somebody's request.
 *
 * The runs are typed in rather than searched for here, and that is deliberate:
 * nothing in this platform indexes content by the person it concerns, because
 * an index of who appears in what would be the very record a subject is asking
 * to be rid of. Finding the runs is done in the trail, which is where the
 * question "what did we do about this person" belongs.
 *
 * It asks for a reason and will not proceed without one. An erasure nobody can
 * account for is indistinguishable from data loss.
 */
export function EraseDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const erase = useEraseContent();
  const [runs, setRuns] = useState("");
  const [reason, setReason] = useState("");

  const named = runs
    .split(/[\s,]+/)
    .map((r) => r.trim())
    .filter(Boolean);

  const submit = () =>
    erase.mutate(
      { runs: named, reason },
      {
        onSuccess: (result) => {
          toast.success(t("retention.erased", { count: result.objects }));
          onClose();
        },
        onError: (e) =>
          toast.error(t("retention.eraseFailed"), {
            description: e instanceof Error ? e.message : undefined,
          }),
      },
    );

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("retention.eraseTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("retention.eraseExplains")}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="erase-runs">{t("retention.runs")}</Label>
          <Textarea
            id="erase-runs"
            value={runs}
            onChange={(e) => setRuns(e.target.value)}
            className="min-h-24 font-mono text-xs"
            spellCheck={false}
            placeholder="run_suporte_1786..."
          />
          <p className="text-xs text-muted-foreground">
            {t("retention.runsHint", { count: named.length })}
          </p>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="erase-reason">{t("retention.reason")}</Label>
          <Input
            id="erase-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={t("retention.reasonPlaceholder")}
          />
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={
              named.length === 0 || reason.trim() === "" || erase.isPending
            }
            onClick={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            {t("retention.eraseConfirm", { count: named.length })}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
