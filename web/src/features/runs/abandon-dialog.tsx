import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { CompensationPlan } from "@/features/runs/compensation-plan";
import {
  useAbandonRun,
  useCompensationPlan,
} from "@/features/runs/compensation-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Ending a run, and undoing what it left standing (PRD SE-08).
 *
 * The plan loads with the dialog, before anything can be pressed. Compensation
 * calls real tools against real systems, and nobody should find out what those
 * were by clicking the button that runs them.
 */
export function AbandonDialog({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [compensate, setCompensate] = useState(true);

  const plan = useCompensationPlan(runId, open);
  const abandon = useAbandonRun(runId);

  async function submit() {
    try {
      await abandon.mutateAsync({ reason, compensate });
      toast.success(t("compensation.abandoned"), {
        description: compensate
          ? t("compensation.abandonedUndoing")
          : t("compensation.abandonedAsIs"),
      });
      setOpen(false);
    } catch (error) {
      toast.error(
        problemMessage(error, t),
      );
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          {t("compensation.abandon")}
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("compensation.abandonTitle")}</DialogTitle>
          <DialogDescription>
            {t("compensation.abandonHint")}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label>{t("compensation.wouldUndo")}</Label>
            <CompensationPlan
              acts={plan.data?.acts}
              loading={plan.isLoading}
            />
          </div>

          <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
            <div>
              <Label htmlFor="compensate">{t("compensation.undoIt")}</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("compensation.undoItHint")}
              </p>
            </div>
            <Switch
              id="compensate"
              checked={compensate}
              onCheckedChange={setCompensate}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="reason">{t("compensation.why")}</Label>
            <Input
              id="reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("compensation.whyPlaceholder")}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          {/* The reason is required by the contract: a run that ended with no
              record of why reads, years later, as one that ended by itself. */}
          <Button
            variant="destructive"
            disabled={abandon.isPending || reason.trim() === ""}
            onClick={() => void submit()}
          >
            {t("compensation.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
