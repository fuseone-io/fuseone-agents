import { useState } from "react";
import { Plug, Plus, Server } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { ServerForm } from "@/features/admin/server-form";
import { ProviderForm } from "@/features/admin/provider-form";
import { RemoveButton } from "@/features/admin/remove-button";
import {
  useDeleteMCPServer,
  useDeleteProvider,
  useIntegrations,
  type MCPServer,
  type ModelProvider,
} from "@/features/admin/api";

type Editing = { kind: "server"; value: MCPServer | null } | { kind: "provider"; value: ModelProvider | null };

export function IntegrationsPanel() {
  const { data, isLoading, error, refetch } = useIntegrations();
  const [editing, setEditing] = useState<Editing | null>(null);

  if (isLoading) return <LoadingRows rows={3} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const servers = data?.mcpServers ?? [];
  const providers = data?.providers ?? [];

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Panel
        title="Servidores de ferramentas"
        action={
          <Button size="sm" onClick={() => setEditing({ kind: "server", value: null })}>
            <Plus className="size-4" />
            Novo
          </Button>
        }
      >
        {servers.length === 0 ? (
          <EmptyState
            icon={<Server className="size-6" />}
            title="Nenhum servidor configurado"
            hint="Um servidor MCP é o que dá ferramentas aos agentes. Enquanto não houver um, os agentes só conseguem raciocinar."
          />
        ) : (
          <ul className="flex flex-col gap-2">
            {servers.map((server) => (
              <ServerRow key={server.name} server={server} onEdit={() => setEditing({ kind: "server", value: server })} />
            ))}
          </ul>
        )}
      </Panel>

      <Panel
        title="Provedores de modelo"
        action={
          <Button size="sm" onClick={() => setEditing({ kind: "provider", value: null })}>
            <Plus className="size-4" />
            Novo
          </Button>
        }
      >
        {providers.length === 0 ? (
          <EmptyState
            icon={<Plug className="size-6" />}
            title="Nenhum provedor configurado"
            hint="Sem provedor, nenhuma execução avança: o agente não tem com o que planejar."
          />
        ) : (
          <ul className="flex flex-col gap-2">
            {providers.map((provider) => (
              <ProviderRow
                key={provider.name}
                provider={provider}
                onEdit={() => setEditing({ kind: "provider", value: provider })}
              />
            ))}
          </ul>
        )}
      </Panel>

      {editing?.kind === "server" && (
        <ServerForm server={editing.value} onClose={() => setEditing(null)} />
      )}
      {editing?.kind === "provider" && (
        <ProviderForm provider={editing.value} onClose={() => setEditing(null)} />
      )}
    </div>
  );
}

function ServerRow({ server, onEdit }: { server: MCPServer; onEdit: () => void }) {
  const remove = useDeleteMCPServer();

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <button
        type="button"
        onClick={onEdit}
        className="min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:underline"
      >
        <div className="font-medium">{server.name}</div>
        <Mono dim>{[server.command, ...(server.args ?? [])].join(" ")}</Mono>
      </button>
      <EnabledBadge enabled={server.enabled} />
      <RemoveButton
        title={`Remover ${server.name}?`}
        description="Os agentes perdem as ferramentas deste servidor no próximo restart do worker. Fica registrado na trilha."
        onConfirm={() =>
          remove.mutate(server.name, {
            onSuccess: () => toast.success(`${server.name} removido`),
            onError: (e) => toast.error(e instanceof Error ? e.message : "Não foi possível remover"),
          })
        }
      />
    </li>
  );
}

function ProviderRow({ provider, onEdit }: { provider: ModelProvider; onEdit: () => void }) {
  const remove = useDeleteProvider();

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <button
        type="button"
        onClick={onEdit}
        className="min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:underline"
      >
        <div className="font-medium">{provider.name}</div>
        <Mono dim>{provider.baseUrl}</Mono>
      </button>
      {/* Whether a credential exists, never what it is. */}
      <Badge variant="secondary" className="font-normal">
        {provider.hasKey ? "com credencial" : "sem credencial"}
      </Badge>
      <EnabledBadge enabled={provider.enabled} />
      <RemoveButton
        title={`Remover ${provider.name}?`}
        description="Agentes que apontam para este provedor param de avançar. A credencial guardada é apagada junto."
        onConfirm={() =>
          remove.mutate(provider.name, {
            onSuccess: () => toast.success(`${provider.name} removido`),
            onError: (e) => toast.error(e instanceof Error ? e.message : "Não foi possível remover"),
          })
        }
      />
    </li>
  );
}

function EnabledBadge({ enabled }: { enabled: boolean }) {
  return (
    <Badge variant="outline" className={enabled ? "text-success" : "text-muted-foreground"}>
      {enabled ? "ativo" : "desativado"}
    </Badge>
  );
}
