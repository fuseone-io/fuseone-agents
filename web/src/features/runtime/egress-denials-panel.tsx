import { ShieldAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { failureLabel } from "@/features/runtime/failure-labels";
import { formatRelative } from "@/lib/format";
import type { RuntimeEgressDenialBucket } from "@/lib/api/client";

export function EgressDenialsPanel({
  denials,
}: {
  denials: RuntimeEgressDenialBucket[];
}) {
  const { t } = useTranslation();
  return (
    <Panel title={t("runtime.egressDenials")} flush>
      {denials.length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">
          {t("runtime.noEgressDenials")}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>{t("runtime.cause")}</TableHead>
              <TableHead className="text-right">{t("runtime.attempts")}</TableHead>
              <TableHead className="text-right">{t("runtime.servers")}</TableHead>
              <TableHead className="text-right">{t("runtime.destinations")}</TableHead>
              <TableHead className="text-right">{t("runtime.lastSeen")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {denials.map((denial) => (
              <TableRow key={denial.code}>
                <TableCell>
                  <div className="flex min-w-0 items-center gap-2">
                    <ShieldAlert className="size-4 shrink-0 text-warning" />
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {t(failureLabel(denial.code))}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {t("runtime.firstSeen", {
                          seen: formatRelative(denial.firstAt),
                        })}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {denial.attempts}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {denial.servers}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {denial.destinations}
                </TableCell>
                <TableCell className="text-right text-muted-foreground">
                  {formatRelative(denial.lastAt)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
