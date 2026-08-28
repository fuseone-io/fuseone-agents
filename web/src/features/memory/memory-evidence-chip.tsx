import { useTranslation } from "react-i18next";
import { ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { shortHash } from "@/lib/format";
import type { Citation } from "@/features/runs/run-citations";

/**
 * The citation a memory will carry, shown and not offered for editing.
 *
 * Read, not typed. Every part of it is the ledger's answer — the step, the
 * artifact, the digest — and a field a person can change is a field they can
 * change to something the run never produced. The server refuses that, so an
 * editable box here would only ever be a way to reach a refusal.
 *
 * The labels are the point of showing it at all. They are what the run had
 * accumulated by this step, so a fact learned inside a poisoned run is marked
 * before somebody decides to teach it rather than after.
 */
export function MemoryEvidenceChip({
  runId,
  citation,
  labels,
}: {
  runId: string;
  citation: Citation;
  /** Null while the trail has not reached the cited step. */
  labels: string[] | null;
}) {
  const { t } = useTranslation();
  return (
    <section
      aria-label={t("memory.evidence")}
      className="rounded-md border bg-muted/40 px-3 py-2"
    >
      <p className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("memory.evidence")}
      </p>
      <Mono className="mt-1 block truncate text-xs">{cited(runId, citation)}</Mono>
      <p className="mt-1 text-2xs text-muted-foreground">
        {t("memory.evidenceFixed")}
      </p>
      <EvidenceLabels labels={labels} />
    </section>
  );
}

/**
 * The citation on one line, assembled here rather than in JSX. The separators
 * are punctuation between machine-produced values, not words, so they belong
 * beside the values instead of in the catalogue.
 */
function cited(runId: string, citation: Citation): string {
  return `${runId} · #${citation.seq} · ${citation.artifact} · ${shortHash(citation.digest)}`;
}

function EvidenceLabels({ labels }: { labels: string[] | null }) {
  const { t } = useTranslation();
  // Absent is not the same as none, and saying "no labels" while the trail is
  // still arriving would understate the taint of the very thing being taught.
  if (labels === null) {
    return <Skeleton className="mt-2 h-5 w-40" aria-label={t("memory.labelsReading")} />;
  }
  return (
    <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1">
      <ShieldAlert className="size-3 shrink-0 text-muted-foreground" aria-hidden />
      {labels.length === 0 ? (
        <span className="text-2xs text-muted-foreground">
          {t("memory.labelsNone")}
        </span>
      ) : (
        labels.map((label) => (
          <Badge key={label} variant="outline" className="font-mono text-2xs">
            {label}
          </Badge>
        ))
      )}
    </div>
  );
}
