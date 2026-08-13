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
import { useSetStop, useStops } from "@/features/admin/stops-api";

/** "" is the whole installation; anything else is `company/area`. */
const EVERYTHING = "";

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

  // Nothing to offer while the installation is already off.
  if (stops?.some((s) => s.level === "installation")) return null;

  const [company = "", area = ""] = target.split("/");
  const stop = () =>
    set.mutate(
      target === EVERYTHING
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
            error instanceof Error ? error.message : t("stops.stopFailed"),
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
            <Select value={target} onValueChange={setTarget}>
              <SelectTrigger id="level">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={EVERYTHING}>
                  {t("stops.levelInstallation")}
                </SelectItem>
                {(scopes?.items ?? []).map((scope) => (
                  <SelectItem
                    key={`${scope.company}/${scope.area}`}
                    value={`${scope.company}/${scope.area}`}
                  >
                    {t("stops.levelScope", {
                      where: scope.label || scope.area,
                    })}
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
