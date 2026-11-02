import { useTranslation } from "react-i18next";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { formatInstant, formatMicros } from "@/lib/format";
import type { WorkItemRole } from "@/lib/api/client";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

const COLUMNS = [
  ["workItems.columnRole", false],
  ["workItems.columnRuns", true],
  ["workItems.columnRequested", true],
  ["workItems.columnApproved", true],
  ["workItems.columnRefused", true],
  ["workItems.columnAbandoned", true],
  ["workItems.columnFirstClaim", true],
  ["workItems.columnLastEnd", true],
  ["workItems.columnCost", true],
] as const;

/**
 * One row per role that worked on the card. The counts are the server's,
 * over every run the reader may see; a cost the reader may not see is
 * absent, and says so, rather than reading as free.
 */
export function WorkItemRoles({ roles }: { roles: WorkItemRole[] }) {
  const { t } = useTranslation();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          {COLUMNS.map(([key, numeric]) => (
            <TableHead
              key={key}
              className={numeric ? `${HEAD} text-right` : HEAD}
            >
              {t(key)}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {roles.map((role) => (
          <TableRow key={role.agentId} className="h-10 border-border-subtle">
            <TableCell className="font-medium">{role.agentId}</TableCell>
            <TableCell className={NUM}>{role.runs}</TableCell>
            <TableCell className={NUM}>{role.approvalsRequested}</TableCell>
            <TableCell className={NUM}>{role.approved}</TableCell>
            <TableCell className={NUM}>{role.refused}</TableCell>
            <TableCell className={NUM}>{role.abandoned}</TableCell>
            <TableCell className={NUM}>
              {formatInstant(role.firstClaimedAt)}
            </TableCell>
            <TableCell className={NUM}>
              {role.lastEndedAt
                ? formatInstant(role.lastEndedAt)
                : t("workItems.notEnded")}
            </TableCell>
            <TableCell className={NUM}>
              <RoleCost micros={role.costMicros} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function RoleCost({ micros }: { micros?: number }) {
  const { t } = useTranslation();
  if (micros !== undefined) return <>{formatMicros(micros)}</>;
  return (
    <Mono dim>
      <span aria-hidden>—</span>
      <span className="sr-only">{t("workItems.costHidden")}</span>
    </Mono>
  );
}
