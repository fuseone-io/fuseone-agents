import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useSetStop, type Stop } from "@/features/admin/stops-api";

/**
 * Takes a switch off again.
 *
 * Behind a confirmation, unlike stopping. The two mistakes are not symmetric:
 * stopping by accident makes the platform go quiet, which is loud and
 * reversible in one press, while starting by accident releases every schedule
 * that has been waiting.
 */
export function StartButton({ stop }: { stop: Stop }) {
  const { t } = useTranslation();
  const set = useSetStop();

  const start = () =>
    set.mutate(
      {
        level: stop.level,
        stopped: false,
        scope: stop.scope,
        agentId: stop.agentId,
      },
      {
        onSuccess: () => toast.success(t("stops.started")),
        onError: (error: unknown) =>
          toast.error(
            error instanceof Error ? error.message : t("stops.startFailed"),
          ),
      },
    );

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-7 shrink-0">
          {t("stops.start")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("stops.startTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("stops.startHelp")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction disabled={set.isPending} onClick={start}>
            {t("stops.start")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
