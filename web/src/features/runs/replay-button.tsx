import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation } from "@tanstack/react-query";
import { History } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ReplayReport } from "@/features/runs/replay-report";
import { api, unwrap } from "@/lib/api/client";

/**
 * Would these decisions be made the same way again? (PRD AU-07)
 *
 * Beside verification, because they are the two halves of one question.
 * Verifying proves the steps were not edited; replaying proves they were the
 * answer the rules in force actually give. A chain of well-sealed lies passes
 * the first and fails the second.
 *
 * On request rather than on load: it re-evaluates every decision in the run,
 * and nobody opening a trail is asking this yet.
 */
export function ReplayButton({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const replay = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.GET("/runs/{runId}/replay", { params: { path: { runId } } }),
      ),
    onSuccess: () => setOpen(true),
  });

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="h-8"
        disabled={replay.isPending}
        onClick={() => replay.mutate()}
      >
        <History className="size-4" aria-hidden />
        {t("runs.replay")}
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("runs.replayTitle")}</DialogTitle>
            <DialogDescription>{t("runs.replayHelp")}</DialogDescription>
          </DialogHeader>
          {replay.data && <ReplayReport report={replay.data} />}
        </DialogContent>
      </Dialog>
    </>
  );
}
