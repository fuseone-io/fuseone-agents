import { useTranslation } from "react-i18next";
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

const PERIOD: Record<string, string> = {
  daily: "admin.perDay",
  monthly: "admin.perMonth",
};

export function BudgetsPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useBudgets();
  const [editing, setEditing] = useState<ScopeBudget | null | undefined>();
  const budgets = data?.items ?? [];

  return (
    <Panel
      title={t("admin.budgets")}
      action={
        <Button size="sm" onClick={() => setEditing(null)}>
          <Plus className="size-4" />
          {t("common.new")}
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
          title={t("admin.noCeiling")}
          hint={t("admin.noCeilingHint")}
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
  const { t } = useTranslation();
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
          {budget.micros
            ? formatMicros(budget.micros)
            : t("admin.noAmountCeiling")}
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
        description={t("admin.removeCeiling")}
        onConfirm={() =>
          remove.mutate(scopePath(budget), {
            onSuccess: () => toast.success(t("admin.ceilingRemoved")),
            onError: (e) =>
              toast.error(
                e instanceof Error ? e.message : t("common.removeFailed"),
              ),
          })
        }
      />
    </li>
  );
}
