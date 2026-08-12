import { useState } from "react";
import { Gauge, Plus } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { RemoveButton } from "@/components/shared/remove-button";
import { BudgetForm } from "@/features/admin/budget-form";
import { scopeLabel, scopePath } from "@/features/admin/budget-scope";
import {
  useBudgets,
  useDeleteBudget,
  type ScopeBudget,
} from "@/features/admin/api";
import { formatMicros } from "@/lib/format";

const PERIOD: Record<string, string> = { daily: "por dia", monthly: "por mês" };

export function BudgetsPanel() {
  const { data, isLoading, error, refetch } = useBudgets();
  const [editing, setEditing] = useState<ScopeBudget | null | undefined>();
  const budgets = data?.items ?? [];

  return (
    <Panel
      title="Tetos"
      action={
        <Button size="sm" onClick={() => setEditing(null)}>
          <Plus className="size-4" />
          Novo
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : budgets.length === 0 ? (
        <EmptyState
          icon={<Gauge className="size-6" />}
          title="Nenhum teto configurado"
          hint="Sem teto por escopo, o único limite é o que cada especificação de agente define por execução — ninguém consegue limitar uma área. Ao atingir um teto a execução pausa e continua depois."
        />
      ) : (
        <ul className="flex flex-col gap-2">
          {budgets.map((budget) => (
            <BudgetRow
              key={scopePath(budget)}
              budget={budget}
              onEdit={() => setEditing(budget)}
            />
          ))}
        </ul>
      )}

      {editing !== undefined && (
        <BudgetForm budget={editing} onClose={() => setEditing(undefined)} />
      )}
    </Panel>
  );
}

function BudgetRow({
  budget,
  onEdit,
}: {
  budget: ScopeBudget;
  onEdit: () => void;
}) {
  const remove = useDeleteBudget();

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <button
        type="button"
        onClick={onEdit}
        className="min-w-0 flex-1 text-left focus-visible:underline focus-visible:outline-none"
      >
        <div className="font-medium">{scopeLabel(budget)}</div>
        <Mono dim>
          {budget.micros ? formatMicros(budget.micros) : "sem teto de valor"}
          {budget.steps ? ` · ${budget.steps} passos` : ""} ·{" "}
          {PERIOD[budget.period]}
        </Mono>
      </button>
      <Badge
        variant="outline"
        className={budget.enabled ? "text-success" : "text-muted-foreground"}
      >
        {budget.enabled ? "ativo" : "desativado"}
      </Badge>
      <RemoveButton
        title={`Remover o teto de ${scopeLabel(budget)}?`}
        description="As execuções desse escopo passam a ser limitadas apenas pelo teto por execução de cada agente. Fica registrado na trilha."
        onConfirm={() =>
          remove.mutate(scopePath(budget), {
            onSuccess: () => toast.success("Teto removido"),
            onError: (e) =>
              toast.error(
                e instanceof Error ? e.message : "Não foi possível remover",
              ),
          })
        }
      />
    </li>
  );
}
