import { useState } from "react";
import { CheckCheck } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { DecisionCard } from "@/features/approvals/decision-card";
import { DecisionPanel } from "@/features/approvals/decision-panel";
import { useApprovals } from "@/features/approvals/api";

export function ApprovalsPage() {
  const { data, isLoading, error, refetch } = useApprovals();
  const items = data?.items ?? [];
  const [selectedRun, setSelectedRun] = useState<string>();

  // Derived, not synchronised: the state holds what somebody clicked, and the
  // fallback handles the rest. Deciding removes an item from the queue and the
  // panel follows on the next render, which is how the same action does not
  // get approved twice from a panel showing a decision already made.
  const selected = items.find((item) => item.runId === selectedRun) ?? items[0];

  return (
    <>
      <PageHeader
        title="Fila humana"
        description="Passos em que um agente parou porque uma pessoa precisa decidir. A decisão fica na trilha, com quem a tomou."
      />

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<CheckCheck className="size-6" />}
          title="Nada aguardando você"
          hint="Quando um agente propuser uma ação que exige decisão humana, ela aparece aqui com o motivo e o que a ação faz."
        />
      ) : (
        <div className="flex min-h-0 flex-1 gap-4">
          <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-auto">
            {items.map((item) => (
              <DecisionCard
                key={`${item.runId}-${item.atSeq}`}
                item={item}
                selected={item.runId === selected?.runId}
                onSelect={() => setSelectedRun(item.runId)}
              />
            ))}
          </div>

          {selected && (
            <DecisionPanel
              key={`${selected.runId}-${selected.atSeq}`}
              item={selected}
              onDecided={() => setSelectedRun(undefined)}
            />
          )}
        </div>
      )}
    </>
  );
}
