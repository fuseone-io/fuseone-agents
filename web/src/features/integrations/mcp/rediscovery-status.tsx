import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";
import {
  useMCPRediscovery,
  type MCPRediscovery,
  type MCPRediscoveryReceipt,
} from "@/features/integrations/mcp/rediscovery-api";

type Answer = "answered" | "failed" | "behind";

// Behind before failed: a failure is about an older request, and what matters
// now is that the latest one has not been answered.
function answerOf(receipt: MCPRediscoveryReceipt, generation: number): Answer {
  if (receipt.generation < generation) return "behind";
  return receipt.succeeded ? "answered" : "failed";
}

const badge: Record<Answer, "secondary" | "destructive" | "outline"> = {
  answered: "secondary",
  failed: "destructive",
  behind: "outline",
};

/**
 * Who answered the latest request, as far as the recorded answers say. They
 * are not a record of which workers are running: one that stopped keeps the
 * last answer it recorded, and one on an older version records none and is
 * absent. The screen says so rather than more than it knows.
 */
export function RediscoveryStatus({ server }: { server: string }) {
  const { t } = useTranslation();
  const read = useMCPRediscovery(server);

  if (read.isLoading) return <LoadingRows rows={2} />;
  if (read.error) return <ErrorState error={read.error} onRetry={() => void read.refetch()} />;
  const status = read.data;
  if (!status || status.generation === 0) {
    return <EmptyState title={t("mcp.rediscovery.noneTitle")} hint={t("mcp.rediscovery.noneHint")} />;
  }
  return (
    <div className="space-y-2">
      <p className="text-sm">
        {t("mcp.rediscovery.latest", {
          by: status.requestedBy ?? "",
          when: status.requestedAt ? formatRelative(status.requestedAt) : "",
        })}
      </p>
      <Receipts status={status} />
      <p className="text-xs text-muted-foreground">{t("mcp.rediscovery.notPresence")}</p>
    </div>
  );
}

function Receipts({ status }: { status: MCPRediscovery }) {
  const { t } = useTranslation();
  if (status.receipts.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("mcp.rediscovery.noWorkers")}</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("mcp.rediscovery.worker")}</TableHead>
          <TableHead>{t("mcp.rediscovery.answer")}</TableHead>
          <TableHead>{t("mcp.rediscovery.when")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {status.receipts.map((receipt) => {
          const answer = answerOf(receipt, status.generation);
          return (
            <TableRow key={receipt.worker}>
              <TableCell><Mono>{receipt.worker}</Mono></TableCell>
              <TableCell>
                <Badge variant={badge[answer]}>{t(`mcp.rediscovery.answers.${answer}`)}</Badge>
              </TableCell>
              <TableCell className="text-muted-foreground">{formatRelative(receipt.at)}</TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
