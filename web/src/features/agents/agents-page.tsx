import { useMemo, useState } from "react";
import { Bot } from "lucide-react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Toolbar } from "@/components/shared/toolbar";
import { FilterSelect, type FilterOption } from "@/components/shared/filter-select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentCard } from "@/features/agents/agent-card";
import { useAgents, type Agent } from "@/features/agents/api";
import { stateOfAgent, type AgentState } from "@/lib/agent-state";

export function AgentsPage() {
  const [history, setHistory] = useState(false);
  const [search, setSearch] = useState("");
  const [area, setArea] = useState("all");
  const [state, setState] = useState<AgentState | "all">("all");

  const { data, isLoading, error, refetch } = useAgents(history);
  const agents = useMemo(() => data?.items ?? [], [data]);

  // Filtered here rather than at the API, and that is defensible only because
  // this list is not paginated: what the browser holds is the whole answer.
  // The runs list is paginated, which is why its search is the server's.
  const areas = useMemo(
    () => [...new Set(agents.map((a) => a.scope.area))].sort(),
    [agents],
  );
  const shown = useMemo(
    () => agents.filter(matcher(search, area, state)),
    [agents, search, area, state],
  );

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.agents}
        title="Agentes"
        description="Cada versão publicada é imutável: o identificador é o resumo do conteúdo, então o texto que uma execução rodou pode sempre ser lido de volta."
      >
        <Tabs value={history ? "all" : "latest"} onValueChange={(v) => setHistory(v === "all")}>
          <TabsList>
            <TabsTrigger value="latest">Atuais</TabsTrigger>
            <TabsTrigger value="all">Histórico</TabsTrigger>
          </TabsList>
        </Tabs>
        <Button size="sm" asChild>
          <Link to="/agents/new">
            <Plus className="size-4" aria-hidden />
            Novo agente
          </Link>
        </Button>
      </PageHeader>

      <Toolbar placeholder="Buscar por nome ou identificador" value={search} onChange={setSearch}>
        <FilterSelect
          label="Filtrar por área"
          value={area}
          options={[
            { value: "all", label: "Todas as áreas" },
            ...areas.map((a) => ({ value: a, label: a })),
          ]}
          onChange={setArea}
          width={170}
        />
        <FilterSelect
          label="Filtrar por estado"
          value={state}
          options={STATES}
          onChange={(v) => setState(v as AgentState | "all")}
          width={190}
        />
      </Toolbar>

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : shown.length === 0 ? (
        <EmptyState
          icon={<Bot className="size-6" />}
          title={agents.length === 0 ? "Nenhum agente publicado" : "Nada encontrado"}
          hint={
            agents.length === 0
              ? "Agentes aparecem aqui quando um worker publica as definições que carregou. Cada publicação cria uma versão nova; a anterior continua legível."
              : "Nenhum agente com esse nome, nessa área e nesse estado."
          }
        />
      ) : (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(268px,1fr))]">
          {shown.map((agent) => (
            <AgentCard key={`${agent.agentId}@${agent.versionId}`} agent={agent} />
          ))}
        </div>
      )}
    </>
  );
}

// An agent has no state of its own — the platform has no autonomy stage yet.
// These are the states of its runs, which is what an operator is actually
// asking about: what is going, what is stuck, what has never run.
const STATES: FilterOption[] = [
  { value: "all", label: "Todos os estados" },
  { value: "running", label: "Em execução" },
  { value: "waiting", label: "Esperando pessoa" },
  { value: "blocked", label: "Estacionado" },
  { value: "done", label: "Última concluída" },
  { value: "draft", label: "Nunca executou" },
];

function matcher(search: string, area: string, state: AgentState | "all") {
  const q = search.trim().toLowerCase();
  return (agent: Agent) => {
    if (area !== "all" && agent.scope.area !== area) return false;
    if (state !== "all" && stateOfAgent(agent.activity?.lastPhase) !== state) return false;
    if (!q) return true;
    return (
      agent.name.toLowerCase().includes(q) || agent.agentId.toLowerCase().includes(q)
    );
  };
}
