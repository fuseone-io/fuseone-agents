import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

/**
 * What a form says when its save was refused because somebody wrote first.
 *
 * The form stays as it is: the person's edits are theirs, and throwing them
 * away to show the newer version would punish them for the other writer.
 * Reloading is offered, and it says plainly that it replaces those edits.
 */
export function StaleNotice({
  onReload,
  reloading = false,
}: {
  onReload: () => void;
  reloading?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <Alert variant="destructive">
      <AlertTitle>{t("revision.staleTitle")}</AlertTitle>
      <AlertDescription className="flex flex-col items-start gap-2">
        <span>{t("revision.staleBody")}</span>
        <Button
          // It sits inside the form it reloads. Left to submit, reloading would
          // also save the draft it is meant to throw away.
          type="button"
          variant="outline"
          size="sm"
          onClick={onReload}
          disabled={reloading}
        >
          <RefreshCw className="size-4" />
          {t("revision.reload")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
