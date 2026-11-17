import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import type { Landed, SaveState } from "@/features/workbench/use-save-draft";

/**
 * What the panel says when a save was refused: by an older ETag, or by a
 * draft that is no longer there. Either way the edits stay on screen; reading
 * again keeps them over what is there now.
 */
export function WorkbenchNotice({
  state,
  landed,
  onReread,
}: {
  state: SaveState;
  /** How much of the refused save had already landed. */
  landed: Landed;
  onReread: () => void;
}) {
  const { t } = useTranslation();
  if (state !== "stale" && state !== "gone") return null;
  const stale = state === "stale";

  return (
    <Alert variant="destructive">
      <AlertTitle>{t(stale ? "workbench.staleTitle" : "workbench.goneTitle")}</AlertTitle>
      <AlertDescription className="flex flex-col items-start gap-2">
        <span>
          {landed.landed > 0
            ? t(stale ? "workbench.stalePartial" : "workbench.gonePartial", landed)
            : t(stale ? "workbench.staleBody" : "workbench.goneBody")}
        </span>
        <Button type="button" variant="outline" size="sm" onClick={onReread}>
          <RefreshCw className="size-4" />
          {t(stale ? "workbench.reread" : "workbench.retry")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
