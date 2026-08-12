import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Scale } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { PoliciesTable } from "@/features/policies/policies-table";
import { tallyOf } from "@/features/policies/policy-tally";
import { usePolicies } from "@/features/policies/api";
import { sinceFor } from "@/features/runs/runs-filters";

/**
 * Every rule the installation runs under.
 *
 * The figures at the top count three states rather than two: off is a
 * decision, watching is a rule that runs and changes nothing, and in force is
 * the only one that stops anything.
 */
export function PoliciesPage() {
  const [period] = useState("7");
  const since = useMemo(() => sinceFor(period), [period]);
  const { data, isLoading, error, refetch } = usePolicies(since);

  const policies = data?.items ?? [];
  const tally = tallyOf(policies);

  return (
    <>
      <PageHeader
        title="Políticas"
        description="As regras que o Portão avalia em todo passo de agente. Uma política em modo monitorar é avaliada, registrada, e não muda nada."
      >
        <Button size="sm" asChild>
          <Link to="/policies/new">
            <Plus className="size-4" aria-hidden />
            Nova política
          </Link>
        </Button>
      </PageHeader>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Figure label="Impondo" value={String(tally.enforcing)} note="param alguma coisa" />
        <Figure
          label="Monitorando"
          value={String(tally.monitoring)}
          note="avaliadas, não obedecidas"
        />
        <Figure label="Negações" value={String(tally.denied)} note="nos últimos 7 dias" />
        <Figure label="Escalações" value={String(tally.escalated)} note="nos últimos 7 dias" />
      </div>

      <Panel
        title="Regras"
        action={
          data?.policyHash ? (
            // The set as a whole has a name, and every decision records it.
            // Showing it is how somebody checks that a worker picked up a
            // change without reading a log.
            <Mono dim className="text-2xs">
              conjunto {data.policyHash.slice(0, 12)}
            </Mono>
          ) : undefined
        }
        flush
      >
        {isLoading ? (
          <div className="p-4">
            <LoadingRows rows={5} />
          </div>
        ) : error ? (
          <div className="p-4">
            <ErrorState error={error} onRetry={() => void refetch()} />
          </div>
        ) : policies.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={<Scale className="size-6" />}
              title="Nenhuma política escrita"
              hint="Sem regras escritas, vale o padrão embutido: leitura passa, escrita pede aprovação humana, e efeito destrutivo ou financeiro é negado. Uma política escrita aperta esse padrão — ou abre uma exceção com o nome de alguém nela."
            />
          </div>
        ) : (
          <PoliciesTable policies={policies} />
        )}
      </Panel>
    </>
  );
}

function Figure({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">{label}</div>
      <div className="mt-1.5 font-mono text-[22px]/7 font-medium tabular-nums">{value}</div>
      <div className="mt-0.5 text-xs text-muted-foreground">{note}</div>
    </div>
  );
}
