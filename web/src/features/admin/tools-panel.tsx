import { useState } from "react";
import { Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { EffectBadge } from "@/features/admin/effect-badge";
import { ClassifyDialog } from "@/features/admin/classify-dialog";
import { useTools, type Tool } from "@/features/admin/api";

const HEAD = "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";

export function ToolsPanel() {
  const { data, isLoading, error, refetch } = useTools();
  const [classifying, setClassifying] = useState<Tool | null>(null);
  const tools = data?.items ?? [];

  return (
    <Panel title="Ferramentas" action={<span className="text-xs text-muted-foreground">chegam como leitura</span>} flush>
      {isLoading ? (
        <div className="p-4">
          <LoadingRows />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : tools.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Wrench className="size-6" />}
            title="Nenhuma ferramenta descoberta"
            hint="Ferramentas aparecem aqui quando um worker conecta em um servidor MCP configurado abaixo e publica o que encontrou."
          />
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className={HEAD}>Ferramenta</TableHead>
              <TableHead className={HEAD}>Servidor</TableHead>
              <TableHead className={HEAD}>Efeito</TableHead>
              <TableHead className={HEAD}>Dado de fora</TableHead>
              <TableHead className={`${HEAD} text-right`} />
            </TableRow>
          </TableHeader>
          <TableBody>
            {tools.map((tool) => (
              <TableRow key={tool.toolId} className="h-10 border-border-subtle">
                <TableCell>
                  <Mono>{tool.toolId}</Mono>
                  {tool.description && (
                    <div className="truncate text-xs text-muted-foreground">{tool.description}</div>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">{tool.server}</TableCell>
                <TableCell>
                  <EffectBadge effect={tool.effect} />
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {tool.untrusted ? "sim" : "não"}
                </TableCell>
                <TableCell className="text-right">
                  <Button variant="outline" size="sm" onClick={() => setClassifying(tool)}>
                    Classificar
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <ClassifyDialog tool={classifying} onClose={() => setClassifying(null)} />
    </Panel>
  );
}
