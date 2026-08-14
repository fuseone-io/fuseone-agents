import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
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
import { Button } from "@/components/ui/button";

/**
 * Reading the instructions again, behind a question.
 *
 * Asked because it replaces the drawing whole. The instructions have not
 * necessarily changed since the stages were arranged, so the ordinary outcome
 * of pressing this by accident is losing an arrangement and getting back the
 * reading it came from.
 */
export function ReadAgainButton({
  steps,
  disabled,
  onConfirm,
}: {
  steps: number;
  disabled: boolean;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="ml-auto h-8"
          disabled={disabled}
        >
          <Sparkles className="size-3.5" aria-hidden />
          {t("agents.readAgain")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents.readAgainTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents.readAgainExplains", { count: steps })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>
            {t("agents.readAgain")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
