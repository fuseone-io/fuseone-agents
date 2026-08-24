import { MessageSquareWarning } from "lucide-react";
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
import type { RuntimeChannelFailureBucket } from "@/lib/api/client";

export function ChannelFailuresPanel({
  failures,
}: {
  failures: RuntimeChannelFailureBucket[];
}) {
  const { t } = useTranslation();
  return (
    <Panel title={t("runtime.channelFailures")} flush>
      {failures.length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">
          {t("runtime.noChannelFailures")}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>{t("runtime.cause")}</TableHead>
              <TableHead className="text-right">{t("runtime.attempts")}</TableHead>
              <TableHead className="text-right">{t("runtime.conversations")}</TableHead>
              <TableHead className="text-right">{t("runtime.runs")}</TableHead>
              <TableHead className="text-right">{t("runtime.lastSeen")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {failures.map((failure) => (
              <TableRow key={failure.code}>
                <TableCell>
                  <div className="flex min-w-0 items-center gap-2">
                    <MessageSquareWarning className="size-4 shrink-0 text-warning" />
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {t(failureLabel(failure.code))}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {t("runtime.firstSeen", {
                          seen: formatRelative(failure.firstAt),
                        })}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {failure.attempts}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  <ConversationCount
                    conversations={failure.conversations}
                    scopeWide={failure.scopeWide}
                  />
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

function ConversationCount({
  conversations,
  scopeWide,
}: {
  conversations: number;
  scopeWide: boolean;
}) {
  const { t } = useTranslation();
  if (!scopeWide) {
    return <>{conversations}</>;
  }
  if (conversations === 0) {
    return <>{t("runtime.wholeScope")}</>;
  }
  return <>{t("runtime.conversationsAndWholeScope", { count: conversations })}</>;
}
