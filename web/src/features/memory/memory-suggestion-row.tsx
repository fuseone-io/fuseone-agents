import { CircleCheck, ShieldAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { MemorySuggestion } from "@/features/memory/api";
import { cn } from "@/lib/utils";

export function MemorySuggestionRow({
  suggestion,
  selected,
  onSelect,
}: {
  suggestion: MemorySuggestion;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const untrusted = suggestion.labels.includes("untrusted");
  return (
    <Button
      type="button"
      variant="ghost"
      aria-pressed={selected}
      onClick={onSelect}
      className={cn(
        "grid h-[70px] w-full min-w-0 justify-stretch rounded-none border-b border-l-[3px] border-b-border-subtle px-3 py-2 text-left whitespace-normal hover:bg-muted/50",
        selected
          ? "border-l-primary bg-surface-accent"
          : "border-l-transparent bg-background",
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {suggestion.subject}
        </span>
        {untrusted && (
          <Badge className="h-5 shrink-0 bg-warning-surface text-warning">
            <ShieldAlert className="mr-1 size-3" aria-hidden />
            {t("memory.outsideData")}
          </Badge>
        )}
        <span className="inline-flex shrink-0 items-center gap-1 font-mono text-2xs text-muted-foreground">
          <CircleCheck className="size-3" aria-hidden />
          {suggestion.observations}
        </span>
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <span className="max-w-[150px] shrink-0 truncate font-mono text-2xs text-text-accent">
          {suggestion.kind}
        </span>
        <span className="size-1 shrink-0 rounded-full bg-border-strong" />
        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {suggestion.claim}
        </span>
      </div>
      <p className="truncate font-mono text-2xs text-muted-foreground">
        {suggestion.signature}
      </p>
    </Button>
  );
}
