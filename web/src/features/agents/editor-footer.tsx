import { useTranslation } from "react-i18next";
import { CircleDot, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Mono } from "@/components/shared/mono";
import type { Change } from "@/features/agents/agent-draft";

/**
 * Publishing, and what it will change.
 *
 * Always visible, because the primary action of the screen used to be
 * reachable only after scrolling past everything on it.
 *
 * The diff is summoned rather than displayed. It is a consequence rather than
 * content — it belongs to the act of publishing, not beside the fields — and a
 * summary permanently on screen is a summary nobody reads by the third visit.
 */
export function EditorFooter({
  changes,
  creating,
  publishing,
  ready,
  onPublish,
  onDiscard,
}: {
  changes: Change[];
  creating: boolean;
  publishing: boolean;
  ready: boolean;
  onPublish: () => void;
  onDiscard: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="sticky bottom-0 flex h-14 shrink-0 items-center gap-3 border-t border-border bg-card px-4">
      {changes.length > 0 && (
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="sm" className="h-8 gap-2">
              <CircleDot className="size-3.5 text-warning" aria-hidden />
              {t("agents.unpublishedChanges", { count: changes.length })}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" side="top" className="w-[340px]">
            <ul className="flex flex-col gap-2">
              {changes.map((change) => (
                <li key={change.field} className="flex flex-col gap-0.5">
                  <span className="text-xs font-medium">{t(change.field)}</span>
                  <Mono dim className="text-2xs">
                    {change.from} → {change.to}
                  </Mono>
                </li>
              ))}
            </ul>
            {/* The one thing somebody publishing needs to be sure of, and the
                reason a version is written rather than a record edited. */}
            <p className="mt-3 border-t border-border-subtle pt-2 text-2xs text-muted-foreground">
              {t("agents.runsStayPinned")}
            </p>
          </PopoverContent>
        </Popover>
      )}

      <div className="ml-auto flex items-center gap-2">
        <Button variant="outline" onClick={onDiscard}>
          {t("common.cancel")}
        </Button>
        <Button onClick={onPublish} disabled={publishing || !ready}>
          <Upload className="size-4" aria-hidden />
          {creating ? t("agents.createPaused") : t("agents.publishVersion")}
        </Button>
      </div>
    </div>
  );
}
