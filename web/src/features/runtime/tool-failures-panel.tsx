import { Wrench } from "lucide-react";
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
import type { RuntimeToolFailureBucket } from "@/lib/api/client";

export function ToolFailuresPanel({
  failures,
}: {
  failures: RuntimeToolFailureBucket[];
}) {
  const { t } = useTranslation();
  return (
    <Panel title={t("runtime.toolFailures")} flush>
      {failures.length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">
          {t("runtime.noToolFailures")}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>{t("runtime.cause")}</TableHead>
              <TableHead className="text-right">{t("runtime.calls")}</TableHead>
              <TableHead className="text-right">{t("runtime.runs")}</TableHead>
              <TableHead className="text-right">{t("runtime.lastSeen")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {failures.map((failure) => (
              <TableRow key={failure.code}>
                <TableCell>
                  <div className="flex min-w-0 items-center gap-2">
                    <Wrench className="size-4 shrink-0 text-warning" />
                    <span className="truncate font-medium">
                      {t(failureLabel(failure.code))}
                    </span>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {failure.calls}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {failure.runs}
                </TableCell>
                <TableCell className="text-right text-muted-foreground">
                  {formatRelative(failure.lastAt)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
