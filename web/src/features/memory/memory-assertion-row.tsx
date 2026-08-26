import { ShieldAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { MemoryAssertion } from "@/features/memory/api";
import {
  MEMORY_STATUS_LABELS,
  memoryStatusClass,
} from "@/features/memory/memory-status";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

export function MemoryAssertionRow({
  assertion,
  selected,
  onSelect,
}: {
  assertion: MemoryAssertion;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const untrusted = assertion.labels.includes("untrusted");
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={cn(
        "grid min-w-0 gap-2 rounded-md border px-3 py-2 text-left transition-colors hover:bg-muted/50",
        selected && "border-primary bg-surface-accent",
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <Badge variant="outline" className="shrink-0 font-mono">
          {assertion.kind}
        </Badge>
        <Badge className={cn("shrink-0", memoryStatusClass(assertion.status))}>
          {t(MEMORY_STATUS_LABELS[assertion.status])}
        </Badge>
        {untrusted && (
          <Badge className="shrink-0 bg-warning-surface text-warning">
            <ShieldAlert className="mr-1 size-3" aria-hidden />
            {t("memory.outsideData")}
          </Badge>
        )}
        <span className="ml-auto shrink-0 text-2xs text-muted-foreground">
          {formatRelative(assertion.updatedAt)}
        </span>
      </div>
      <div className="grid min-w-0 gap-1 md:grid-cols-[minmax(0,1fr)_120px_160px]">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{assertion.subject}</p>
          <p className="truncate font-mono text-2xs text-muted-foreground">
            {assertion.signature}
          </p>
        </div>
        <p className="truncate text-xs tabular-nums text-muted-foreground">
          {t("memory.observed")}: {assertion.confirmed}/{assertion.observations}
        </p>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {assertion.agentId || t("memory.shared")}
        </p>
      </div>
      <p className="truncate text-sm text-muted-foreground">
        {assertion.claim}
      </p>
    </button>
  );
}
