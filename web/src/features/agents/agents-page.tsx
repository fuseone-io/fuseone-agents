import { useMemo, useState } from "react";
import { Bot } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Toolbar } from "@/components/shared/toolbar";
import { FilterSelect } from "@/components/shared/filter-select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentCard } from "@/features/agents/agent-card";
import { useAgents, type Agent } from "@/features/agents/api";

export function AgentsPage() {
  const [history, setHistory] = useState(false);
  const [search, setSearch] = useState("");
  const [area, setArea] = useState("all");

  const { data, isLoading, error, refetch } = useAgents(history);
  const agents = useMemo(() => data?.items ?? [], [data]);

  // Filtered here rather than at the API, and that is defensible only because
  // this list is not paginated: what the browser holds is the whole answer.
  // The runs list is paginated, which is why its search is the server's.
  const areas = useMemo(
    () => [...new Set(agents.map((a) => a.scope.area))].sort(),
    [agents],
  );
  const shown = useMemo(() => agents.filter(matcher(search, area)), [agents, search, area]);

  return (
    <>
      <PageHeader
        title="Agentes"
        description="Cada versão publicada é imutável: o identificador é o resumo do conteúdo, então o texto que uma execução rodou pode sempre ser lido de volta."
      >
        <Tabs value={history ? "all" : "latest"} onValueChange={(v) => setHistory(v === "all")}>
          <TabsList>
            <TabsTrigger value="latest">Atuais</TabsTrigger>
            <TabsTrigger value="all">Histórico</TabsTrigger>
          </TabsList>
        </Tabs>
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
          width={180}
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
              : "Nenhum agente com esse nome ou identificador na área selecionada."
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

function matcher(search: string, area: string) {
  const q = search.trim().toLowerCase();
  return (agent: Agent) => {
    if (area !== "all" && agent.scope.area !== area) return false;
    if (!q) return true;
    return (
      agent.name.toLowerCase().includes(q) || agent.agentId.toLowerCase().includes(q)
    );
  };
}
