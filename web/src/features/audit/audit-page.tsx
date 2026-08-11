import { useDeferredValue, useMemo, useState } from "react";
import { ScrollText } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Toolbar } from "@/components/shared/toolbar";
import { FilterSelect, type FilterOption } from "@/components/shared/filter-select";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { AuditTable } from "@/features/audit/audit-table";
import { IntegrityBanner } from "@/features/audit/integrity-banner";
import { useAudit } from "@/features/audit/api";
import { sinceFor } from "@/features/runs/runs-filters";

const SOURCES: FilterOption[] = [
  { value: "all", label: "Os dois registros" },
  { value: "ledger", label: "Ledger — decisões" },
  { value: "admin", label: "Administrativo — mudanças" },
];

const PERIODS: FilterOption[] = [
  { value: "1", label: "Últimas 24h" },
  { value: "7", label: "Últimos 7 dias" },
  { value: "30", label: "Últimos 30 dias" },
  { value: "all", label: "Desde o início" },
];

/**
 * Everything that happened, from both records that keep it.
 *
 * The two are merged because a person asking "what happened" does not care
 * which table it landed in — and separated by their seal, because only one of
 * them can prove it was not altered.
 */
export function AuditPage() {
  const [source, setSource] = useState("all");
  const [period, setPeriod] = useState("7");
  const [actor, setActor] = useState("");
  const query = useDeferredValue(actor.trim());

  const since = useMemo(() => sinceFor(period), [period]);
  const { data, isLoading, error, refetch } = useAudit({
    since,
    actor: query || undefined,
    source: source === "all" ? undefined : (source as "ledger" | "admin"),
  });

  const entries = data?.items ?? [];

  return (
    <>
      <PageHeader
        title="Trilha de auditoria"
        description="O que os agentes fizeram e o que as pessoas mudaram nas regras sob as quais eles rodam. Os dois registros são append-only; só um é encadeado."
      />

      {!isLoading && !error && <IntegrityBanner entries={entries} />}

      <Toolbar placeholder="Quem agiu — pessoa ou agente" value={actor} onChange={setActor}>
        <FilterSelect
          label="Registro"
          value={source}
          options={SOURCES}
          onChange={setSource}
          width={230}
        />
        <FilterSelect
          label="Período"
          value={period}
          options={PERIODS}
          onChange={setPeriod}
          width={180}
        />
      </Toolbar>

      <Panel
        title="Trilha"
        action={<span className="text-xs text-muted-foreground">append-only</span>}
        flush
      >
        {isLoading ? (
          <div className="p-4">
            <LoadingRows rows={8} />
          </div>
        ) : error ? (
          <div className="p-4">
            <ErrorState error={error} onRetry={() => void refetch()} />
          </div>
        ) : entries.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={<ScrollText className="size-6" />}
              title="Nada registrado no período"
              hint="Aparecem aqui as decisões do Portão, as aprovações humanas e toda mudança que alguém fizer nas regras — classificar uma ferramenta, configurar um provedor, definir um teto."
            />
          </div>
        ) : (
          <AuditTable entries={entries} />
        )}
      </Panel>
    </>
  );
}
