import { Link } from "react-router-dom";
import { CheckCheck } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { useApprovals } from "./api";
import { formatRelative } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";

export function ApprovalsPage() {
  const { data, isLoading, error, refetch } = useApprovals();

  if (isLoading) return <LoadingRows />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const items = data?.items ?? [];
  if (items.length === 0) {
    return (
      <EmptyState
        icon={<CheckCheck className="size-6" />}
        title="Nada aguardando você"
        hint="Quando um agente propuser uma ação que exige decisão humana, ela aparece aqui com o motivo."
      />
    );
  }

  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Agente</TableHead>
            <TableHead>Ferramenta</TableHead>
            <TableHead>Motivo</TableHead>
            <TableHead className="text-right">Pedido</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => (
            <TableRow key={`${item.runId}-${item.atSeq}`}>
              <TableCell>
                <Link
                  to={`/runs/${item.runId}`}
                  className="font-medium underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none"
                >
                  {item.agentId ?? item.runId}
                </Link>
                <div className="text-xs text-muted-foreground">{item.scope?.area}</div>
              </TableCell>
              <TableCell>
                <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{item.tool}</code>
              </TableCell>
              <TableCell className="text-muted-foreground" title={item.reason}>
                {explainRule(item.rule)}
              </TableCell>
              <TableCell className="text-right text-muted-foreground">
                {formatRelative(item.requestedAt)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
