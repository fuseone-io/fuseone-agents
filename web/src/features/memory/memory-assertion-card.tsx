import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { MEMORY_STATUS_LABELS, memoryStatusClass } from "@/features/memory/memory-status";
import { formatRelative, shortHash } from "@/lib/format";
import type { MemoryAssertion } from "@/features/memory/api";

export function MemoryAssertionCard({
  assertion,
  canDisable,
  onDisable,
}: {
  assertion: MemoryAssertion;
  canDisable: boolean;
  onDisable: (assertion: MemoryAssertion) => void;
}) {
  const { t } = useTranslation();
  return (
    <article className="min-w-0 rounded-lg border bg-background p-4">
      <header className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Badge variant="outline" className="font-mono">
              {assertion.kind}
            </Badge>
            <Badge className={memoryStatusClass(assertion.status)}>
              {t(MEMORY_STATUS_LABELS[assertion.status])}
            </Badge>
          </div>
          <h2 className="mt-2 truncate text-sm font-medium">
            {assertion.subject}
          </h2>
          <p className="truncate font-mono text-2xs text-muted-foreground">
            {assertion.signature}
          </p>
        </div>
        {canDisable && assertion.status === "active" && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => onDisable(assertion)}
          >
            {t("memory.disable")}
          </Button>
        )}
      </header>

      <p className="mt-3 line-clamp-3 text-sm text-muted-foreground">
        {assertion.claim}
      </p>

      <div className="mt-4 grid gap-2 sm:grid-cols-3">
        <Fact label={t("memory.scope")} value={scopeOf(assertion)} mono />
        <Fact label={t("memory.observed")} value={`${assertion.confirmed}/${assertion.observations}`} />
        <Fact label={t("memory.agent")} value={assertion.agentId || t("memory.shared")} mono />
      </div>

      <EvidenceList assertion={assertion} />

      <footer className="mt-3 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border-t pt-3 text-2xs text-muted-foreground">
        <span>{t("memory.updated", { seen: formatRelative(assertion.updatedAt) })}</span>
        {assertion.labels.length > 0 && (
          <span className="inline-flex min-w-0 items-center gap-1">
            <ShieldAlert className="size-3 shrink-0" aria-hidden />
            <span className="truncate">{assertion.labels.join(", ")}</span>
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

function EvidenceList({ assertion }: { assertion: MemoryAssertion }) {
  const { t } = useTranslation();
  return (
    <div className="mt-3 rounded-md border px-3 py-2">
      <p className="text-2xs uppercase text-muted-foreground">
        {t("memory.evidence")}
      </p>
      <div className="mt-1 grid gap-1">
        {assertion.evidence.map((evidence) => (
          <p
            key={`${evidence.runId}/${evidence.artifact}/${evidence.digest}`}
            className="min-w-0 truncate font-mono text-xs"
          >
            {evidence.runId} · {evidence.artifact} · {shortHash(evidence.digest)}
          </p>
        ))}
      </div>
    </div>
  );
}

function scopeOf(assertion: MemoryAssertion): string {
  return `${assertion.scope.company}/${assertion.scope.area}`;
}
