import { Check, ShieldAlert, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  MEMORY_SUGGESTION_STATUS_LABELS,
  memorySuggestionStatusClass,
} from "@/features/memory/memory-suggestion-status";
import type { MemorySuggestion } from "@/features/memory/api";
import { formatRelative, shortHash } from "@/lib/format";

export function MemorySuggestionCard({
  suggestion,
  canReview,
  onAccept,
  onDismiss,
}: {
  suggestion: MemorySuggestion;
  canReview: boolean;
  onAccept: (suggestion: MemorySuggestion) => void;
  onDismiss: (suggestion: MemorySuggestion) => void;
}) {
  const { t } = useTranslation();
  const pending = suggestion.status === "pending";
  return (
    <article className="min-w-0 rounded-lg border bg-background p-4">
      <header className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Badge variant="outline" className="font-mono">
              {suggestion.kind}
            </Badge>
            <Badge className={memorySuggestionStatusClass(suggestion.status)}>
              {t(MEMORY_SUGGESTION_STATUS_LABELS[suggestion.status])}
            </Badge>
          </div>
          <h2 className="mt-2 truncate text-sm font-medium">
            {suggestion.subject}
          </h2>
          <p className="truncate font-mono text-2xs text-muted-foreground">
            {suggestion.signature}
          </p>
        </div>
        {canReview && pending && (
          <div className="flex shrink-0 items-center gap-1">
            <Button
              type="button"
              size="icon"
              variant="outline"
              aria-label={t("memory.acceptSuggestion")}
              onClick={() => onAccept(suggestion)}
            >
              <Check className="size-4" aria-hidden />
            </Button>
            <Button
              type="button"
              size="icon"
              variant="outline"
              aria-label={t("memory.dismissSuggestion")}
              onClick={() => onDismiss(suggestion)}
            >
              <X className="size-4" aria-hidden />
            </Button>
          </div>
        )}
      </header>

      <p className="mt-3 line-clamp-3 text-sm text-muted-foreground">
        {suggestion.claim}
      </p>

      <div className="mt-4 grid gap-2 sm:grid-cols-3">
        <Fact label={t("memory.scope")} value={scopeOf(suggestion)} mono />
        <Fact
          label={t("memory.observations")}
          value={String(suggestion.observations)}
        />
        <Fact
          label={t("memory.agent")}
          value={suggestion.agentId || t("memory.shared")}
          mono
        />
      </div>

      <div className="mt-3 rounded-md border px-3 py-2">
        <p className="text-2xs uppercase text-muted-foreground">
          {t("memory.evidence")}
        </p>
        <div className="mt-1 grid gap-1">
          {suggestion.evidence.map((evidence) => (
            <p
              key={`${evidence.runId}/${evidence.artifact}/${evidence.digest}`}
              className="min-w-0 truncate font-mono text-xs"
            >
              {evidence.runId} · {evidence.artifact} · {shortHash(evidence.digest)}
            </p>
          ))}
        </div>
      </div>

      <footer className="mt-3 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border-t pt-3 text-2xs text-muted-foreground">
        <span>
          {t("memory.suggested", { seen: formatRelative(suggestion.updatedAt) })}
        </span>
        {suggestion.labels.length > 0 && (
          <span className="inline-flex min-w-0 items-center gap-1">
            <ShieldAlert className="size-3 shrink-0" aria-hidden />
            <span className="truncate">{suggestion.labels.join(", ")}</span>
          </span>
        )}
      </footer>
    </article>
  );
}

function Fact({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0 rounded-md border px-3 py-2">
      <p className="text-2xs uppercase text-muted-foreground">{label}</p>
      <p className={mono ? "truncate font-mono text-xs" : "truncate text-xs"}>
        {value}
      </p>
    </div>
  );
}

function scopeOf(suggestion: MemorySuggestion): string {
  return `${suggestion.scope.company}/${suggestion.scope.area}`;
}
