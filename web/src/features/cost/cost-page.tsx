import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Wallet } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { api, unwrap } from "@/lib/api/client";
import { formatCost, formatTokens } from "@/lib/format";

const MONTH_MS = 30 * 24 * 60 * 60 * 1000;
const HOUR_MS = 60 * 60 * 1000;

export function CostPage() {
  // Rounded to the hour and computed once. Reading the clock during render
  // produced a new `to` on every pass, so the query key never repeated and the
  // page refetched the whole cost rollup continuously.
  const [range] = useState(() => {
    const to = Math.floor(Date.now() / HOUR_MS) * HOUR_MS;
    return { from: new Date(to - MONTH_MS).toISOString(), to: new Date(to).toISOString() };
  });

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["cost", range],
    queryFn: async () =>
      unwrap(await api.GET("/cost", { params: { query: { ...range, groupBy: "agent" } } })),
  });

  if (isLoading) return <LoadingRows />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const buckets = data?.buckets ?? [];
  if (buckets.length === 0) {
    return (
      <EmptyState
        icon={<Wallet className="size-6" />}
        title="Sem consumo no período"
        hint="O custo aparece assim que a primeira execução for concluída. Cada execução é a unidade contábil; agente e área são recortes dela."
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardContent className="px-4 py-3">
          <p className="text-xs text-muted-foreground">Total nos últimos 30 dias</p>
          <p className="mt-0.5 text-2xl font-semibold tabular-nums">{formatCost(data?.total)}</p>
        </CardContent>
      </Card>

      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agente</TableHead>
              <TableHead className="text-right">Execuções</TableHead>
              <TableHead className="text-right">Tokens</TableHead>
              <TableHead className="text-right">Custo</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {buckets.map((b) => (
              <TableRow key={b.key}>
                <TableCell className="font-medium">{b.key}</TableCell>
                <TableCell className="text-right tabular-nums">{b.runs}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatTokens(b.cost.inputTokens)}
                </TableCell>
                <TableCell className="text-right tabular-nums">{formatCost(b.cost)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
