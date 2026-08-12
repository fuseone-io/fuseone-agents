import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Section, Labelled } from "@/features/policies/section";
import { AgentAreaField } from "@/features/agents/agent-area-field";
import type { AgentDefinition } from "@/lib/api/client";

/** Who this agent is, and what it was told to do. */
export function AgentBasicsSection({
  draft,
  patch,
  agentId,
  editable,
  onAgentId,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  agentId: string;
  editable: boolean;
  onAgentId: (id: string) => void;
}) {
  return (
    <Section title="Identidade" hint="A área é a unidade de custo e de política.">
      <div className="grid gap-3 sm:grid-cols-[200px_1fr_160px]">
        <Labelled label="Identificador" htmlFor="agent-id">
          {/* Set once: runs reference it forever, and an id that moved would
              orphan every one of them. */}
          <Input
            id="agent-id"
            value={agentId}
            onChange={(e) => onAgentId(e.target.value)}
            disabled={!editable}
            readOnly={!editable}
            className="font-mono"
            placeholder="suporte"
          />
        </Labelled>
        <Labelled label="Nome" htmlFor="agent-name">
          <Input
            id="agent-name"
            value={draft.name}
            onChange={(e) => patch({ name: e.target.value })}
            placeholder="Atendimento de suporte"
          />
        </Labelled>
        <AgentAreaField area={draft.area} onChange={(area) => patch({ area })} />
      </div>

      <div className="grid gap-3 sm:grid-cols-[1fr_1fr_140px]">
        <Labelled label="Provedor" htmlFor="agent-provider">
          <Input
            id="agent-provider"
            value={draft.provider}
            onChange={(e) => patch({ provider: e.target.value })}
            className="font-mono"
            placeholder="openai"
          />
        </Labelled>
        <Labelled label="Modelo" htmlFor="agent-model">
          <Input
            id="agent-model"
            value={draft.model}
            onChange={(e) => patch({ model: e.target.value })}
            className="font-mono"
          />
        </Labelled>
        <Labelled label="Esforço" htmlFor="agent-effort">
          <Input
            id="agent-effort"
            value={draft.effort ?? ""}
            onChange={(e) => patch({ effort: e.target.value })}
            className="font-mono"
            placeholder="low"
          />
        </Labelled>
      </div>

      <Labelled label="Instruções" htmlFor="agent-instructions">
        <Textarea
          id="agent-instructions"
          rows={8}
          value={draft.instructions}
          onChange={(e) => patch({ instructions: e.target.value })}
          className="font-mono text-xs"
          placeholder="Você atende chamados que chegam em suporte@…"
        />
        <p className="text-xs text-muted-foreground">
          {draft.instructions.trim().length} caracteres. É o texto que a versão
          publicada carrega, e é o que um auditor vai ler para entender uma
          execução daqui a dois anos.
        </p>
      </Labelled>
    </Section>
  );
}
