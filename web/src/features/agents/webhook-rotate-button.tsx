import { useTranslation } from "react-i18next";
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
import type { Webhook } from "@/lib/api/client";

/**
 * Generating the first key breaks nothing. Replacing one that exists breaks
 * every sender configured against it, so only that asks first.
 */
export function RotateButton({
  hook,
  pending,
  onGenerate,
}: {
  hook: Webhook;
  pending: boolean;
  onGenerate: () => void;
}) {
  const { t } = useTranslation();
  if (!hook.armed) {
    return (
      <Button
        variant="outline"
        size="sm"
        className="ml-auto h-7"
        disabled={pending}
        onClick={onGenerate}
      >
        {t("agents.generateKey")}
      </Button>
    );
  }

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="ml-auto h-7"
          disabled={pending}
        >
          {t("agents.rotate")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("agents.rotateTitle", { path: hook.path })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents.rotateWarning")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={onGenerate}>
            {t("agents.rotate")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
