import { useState } from "react";
import { Bot } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentCard } from "@/features/agents/agent-card";
import { useAgents } from "@/features/agents/api";

export function AgentsPage() {
  const [history, setHistory] = useState(false);
  const { data, isLoading, error, refetch } = useAgents(history);
  const agents = data?.items ?? [];

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

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : agents.length === 0 ? (
        <EmptyState
          icon={<Bot className="size-6" />}
          title="Nenhum agente publicado"
          hint="Agentes aparecem aqui quando um worker publica as definições que carregou. Cada publicação cria uma versão nova; a anterior continua legível."
        />
      ) : (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(268px,1fr))]">
          {agents.map((agent) => (
            <AgentCard key={`${agent.agentId}@${agent.versionId}`} agent={agent} />
          ))}
        </div>
      )}
    </>
  );
}
