import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Play } from "lucide-react";
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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useStartRun } from "@/features/agents/start-run";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Opens a run, with what it is about.
 *
 * A dialog rather than a bare button because a run without input is a run the
 * agent has to guess the subject of, and because opening one calls real tools
 * — a step worth taking deliberately rather than by brushing against a
 * control.
 */
export function RunNowDialog({
  agentId,
  agentName,
}: {
  agentId: string;
  agentName: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const navigate = useNavigate();
  const start = useStartRun(agentId);

  const submit = () => {
    start.mutate(input.trim() || undefined, {
      onSuccess: (run) => {
        setOpen(false);
        setInput("");
        toast.success(t("agents.runOpened"), { description: run.runId });
        navigate(`/runs/${run.runId}`);
      },
      onError: (error) =>
        toast.error(t("agents.runFailed"), {
          description: problemMessage(error, t),
        }),
    });
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" className="h-9">
          <Play className="size-4" aria-hidden />
          {t("agents.run")}
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("agents.runNamed", { name: agentName })}</DialogTitle>
          <DialogDescription>
            {t("agents.runOpensOnPublished")}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          <Label htmlFor="run-input">{t("agents.aboutWhat")}</Label>
          <Textarea
            id="run-input"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={t("agents.inputPlaceholder")}
            rows={4}
          />
          <p className="text-xs text-muted-foreground">
            {t("agents.keptOutsideTrail")}
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} disabled={start.isPending}>
            {start.isPending ? "Abrindo…" : t("agents.run")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
