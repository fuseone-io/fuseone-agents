import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { verbOf } from "@/features/audit/audit-verb";
import { formatInstant, shortHash } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { AuditEntry } from "@/lib/api/client";

/**
 * The trail as a table, because an auditor reads down a column.
 *
 * Everything the platform produced is mono; the detail is the one column a
 * person reads as prose. The seal is present only where one exists — an
 * administrative entry shows a dash rather than an empty cell, so the absence
 * reads as a fact rather than as a missing value.
 */
export function AuditTable({ entries }: { entries: AuditEntry[] }) {
  const { t } = useTranslation();
  return (
    <Table>
      <TableHeader>
        <TableRow className="border-border-subtle hover:bg-transparent">
          <TableHead className="text-2xs uppercase tracking-label">
            {t("audit.when")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("audit.who")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("audit.what")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("audit.about")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("audit.detail")}
          </TableHead>
          <TableHead className="text-right text-2xs uppercase tracking-label">
            {t("audit.seal")}
          </TableHead>
        </TableRow>
      </TableHeader>

      <TableBody>
        {entries.map((entry) => (
          <TableRow
            key={`${entry.at}-${entry.source}-${entry.seq ?? 0}-${entry.target}`}
            className="h-10 border-border-subtle"
          >
            <TableCell>
              <Mono dim>{formatInstant(entry.at)}</Mono>
            </TableCell>
            <TableCell>
              <Mono>{entry.actor || "—"}</Mono>
            </TableCell>
            <TableCell>
              <span
                className={cn(
                  "text-xs font-medium",
                  verbOf(entry.verb).className,
                )}
              >
                {verbOf(entry.verb).label}
              </span>
            </TableCell>
            <TableCell>
              <Mono>{entry.target}</Mono>
            </TableCell>
            <TableCell className="max-w-[240px] truncate text-xs text-muted-foreground">
              {detailOf(entry)}
            </TableCell>
            <TableCell className="text-right">
              <Seal entry={entry} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

/**
 * A dash, not a blank: an administrative entry has no seal because that trail
 * is not chained, and an empty cell reads as a value nobody filled in.
 */
function Seal({ entry }: { entry: AuditEntry }) {
  if (!entry.hash) {
    return <span className="text-2xs text-muted-foreground">—</span>;
  }
  if (!entry.runId) {
    return (
      <Mono dim className="text-2xs">
        {shortHash(entry.hash)}
      </Mono>
    );
  }
  return (
    <Link to={`/runs/${entry.runId}`} className="hover:underline">
      <Mono dim className="text-2xs">
        {shortHash(entry.hash)}
      </Mono>
    </Link>
  );
}

/** The one column written for a person to read. */
function detailOf(entry: AuditEntry): string {
  const detail = entry.detail ?? {};
  for (const key of ["reason", "rule", "effect", "note"]) {
    const value = detail[key];
    if (typeof value === "string" && value !== "") return value;
  }
  return entry.scope?.area ? `área ${entry.scope.area}` : "instalação";
}
