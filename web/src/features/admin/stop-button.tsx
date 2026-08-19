import { useState } from "react";
import { useTranslation } from "react-i18next";
import { OctagonX } from "lucide-react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useScopes } from "@/features/scope/api";
import { EVERYTHING, stopTargetsFor } from "@/features/admin/stop-access";
import { useSetStop, useStops } from "@/features/admin/stops-api";
import { useMe } from "@/features/session/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Stops the platform without a deploy (PRD FO-06).
 *
 * In the header of the overview, because the person who needs it is not
 * browsing the administration area — they are looking at the screen that told
 * them something is wrong. One press away, and one confirmation deep, is the
 * right distance for a control whose mistake is recoverable and whose absence
 * is not.
 */
export function StopButton() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState(EVERYTHING);
  const [reason, setReason] = useState("");
  const set = useSetStop();
  const { data: stops } = useStops();
  const { data: scopes } = useScopes();
  const { data: me } = useMe();

  // Nothing to offer while the installation is already off.
  if (stops?.some((s) => s.level === "installation")) return null;

  if (me === undefined) return null;
  const targets = stopTargetsFor(me, scopes?.items ?? []);
  const firstTarget = targets[0];
  if (!firstTarget) return null;

  const selected = targets.some((candidate) => candidate.value === target)
    ? target
    : firstTarget.value;
  const [company = "", area = ""] = selected.split("/");
  const stop = () =>
    set.mutate(
      selected === EVERYTHING
        ? { level: "installation", stopped: true, reason }
        : { level: "scope", stopped: true, reason, scope: { company, area } },
      {
        onSuccess: () => {
          toast.success(t("stops.stoppedNow"), {
            description: t("stops.stoppedHint"),
          });
          setOpen(false);
          setReason("");
        },
        onError: (error: unknown) =>
          toast.error(
            problemMessage(error, t),
          ),
      },
    );

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          <OctagonX className="size-4" aria-hidden />
          {t("stops.stop")}
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("stops.stopTitle")}</DialogTitle>
          <DialogDescription>{t("stops.stopHelp")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="level">{t("stops.howWide")}</Label>
            {/* One agent is the pause on its own screen. This control is for
                the two levels wider than that, because somebody reaching for
                it usually cannot yet name the agent. */}
            <Select value={selected} onValueChange={setTarget}>
              <SelectTrigger id="level">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {targets.map((candidate) => (
                  <SelectItem
                    key={candidate.value || "installation"}
                    value={candidate.value}
                  >
                    {candidate.value === EVERYTHING
                      ? t("stops.levelInstallation")
                      : t("stops.levelScope", { where: candidate.label })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="stopReason">{t("stops.why")}</Label>
            <Input
              id="stopReason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("stops.whyPlaceholder")}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={set.isPending || reason.trim() === ""}
            onClick={stop}
          >
            {t("stops.stop")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
