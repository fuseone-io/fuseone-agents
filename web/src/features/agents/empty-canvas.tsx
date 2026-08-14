import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * An agent with no stages yet.
 *
 * The empty state carries the action rather than describing it, because the
 * canvas is where somebody is already looking — a line of prose here and a
 * button in the header above is how an empty screen stays empty.
 */
export function EmptyCanvas({
  reading,
  canRead,
  onRead,
}: {
  reading: boolean;
  canRead: boolean;
  onRead: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 bg-background p-6 text-center">
      <p className="max-w-sm text-xs text-muted-foreground">
        {canRead ? t("agents.emptyCanvas") : t("agents.writeFirst")}
      </p>
      {canRead && (
        <Button type="button" size="sm" disabled={reading} onClick={onRead}>
          <Sparkles className="size-3.5" aria-hidden />
          {t("agents.readTheInstructions")}
        </Button>
      )}
    </div>
  );
}
