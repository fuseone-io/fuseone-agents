import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { PlayCircle } from "lucide-react";
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
import { api, unwrap } from "@/lib/api/client";
import { runKeys } from "@/features/runs/api";

/**
 * Returns a parked run to the queue (PRD FO-04).
 *
 * Parking means the machine is stuck until a person does something — raises a
 * ceiling, fixes an upstream, widens a pack. This is that person saying they
 * have, and the run continues from the exact step it stopped at.
 *
 * A dialog rather than a bare button because the trail should say what
 * changed. A run that resumed for no stated reason is one nobody can explain
 * later.
 */
export function ResumeButton({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const queryClient = useQueryClient();

  const resume = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/runs/{runId}/resume", {
          params: { path: { runId } },
          body: { note },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: runKeys.all });
      toast.success(t("runs.resumed"), {
        description: t("runs.resumedHint"),
      });
      setOpen(false);
    },
    onError: (error: unknown) =>
      toast.error(
        error instanceof Error ? error.message : t("runs.resumeFailed"),
      ),
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          <PlayCircle className="size-4" aria-hidden />
          {t("runs.resume")}
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("runs.resumeTitle")}</DialogTitle>
          <DialogDescription>{t("runs.resumeHelp")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          <Label htmlFor="note">{t("runs.whatChanged")}</Label>
          <Input
            id="note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t("runs.whatChangedPlaceholder")}
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={resume.isPending || note.trim() === ""}
            onClick={() => resume.mutate()}
          >
            {t("runs.resume")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
