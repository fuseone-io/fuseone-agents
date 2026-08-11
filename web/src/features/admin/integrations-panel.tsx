import { useState } from "react";
import { Plug, Plus, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { ServerForm } from "@/features/admin/server-form";
import { ProviderForm } from "@/features/admin/provider-form";
import { ServerCard } from "@/features/admin/server-card";
import { ProviderCard } from "@/features/admin/provider-card";
import { useIntegrations, type MCPServer, type ModelProvider } from "@/features/admin/api";

type Editing =
  | { kind: "server"; value: MCPServer | null }
  | { kind: "provider"; value: ModelProvider | null };

/**
 * What the platform is connected to, and whether any of it is answering.
 *
 * Cards rather than rows because the state of a connection is the point, and a
 * table makes every system look equally fine until somebody reads the column
 * on the right.
 */
export function IntegrationsPanel() {
  const { data, isLoading, error, refetch } = useIntegrations();
  const [editing, setEditing] = useState<Editing | null>(null);

  if (isLoading) return <LoadingRows rows={3} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const servers = data?.mcpServers ?? [];
  const providers = data?.providers ?? [];

  return (
    <div className="flex flex-col gap-6">
      <Section
        title="Servidores de ferramentas"
        onAdd={() => setEditing({ kind: "server", value: null })}
        empty={
          servers.length === 0 && (
            <EmptyState
              icon={<Server className="size-6" />}
              title="Nenhum servidor configurado"
              hint="Um servidor MCP é o que dá ferramentas aos agentes. Enquanto não houver um, os agentes só conseguem raciocinar."
            />
          )
        }
      >
        {servers.map((server) => (
          <ServerCard
            key={server.name}
            server={server}
            onEdit={() => setEditing({ kind: "server", value: server })}
          />
        ))}
      </Section>

      <Section
        title="Provedores de modelo"
        onAdd={() => setEditing({ kind: "provider", value: null })}
        empty={
          providers.length === 0 && (
            <EmptyState
              icon={<Plug className="size-6" />}
              title="Nenhum provedor configurado"
              hint="Sem provedor, nenhuma execução avança: o agente não tem com o que planejar."
            />
          )
        }
      >
        {providers.map((provider) => (
          <ProviderCard
            key={provider.name}
            provider={provider}
            onEdit={() => setEditing({ kind: "provider", value: provider })}
          />
        ))}
      </Section>

      {editing?.kind === "server" && (
        <ServerForm server={editing.value} onClose={() => setEditing(null)} />
      )}
      {editing?.kind === "provider" && (
        <ProviderForm provider={editing.value} onClose={() => setEditing(null)} />
      )}
    </div>
  );
}

function Section({
  title,
  onAdd,
  empty,
  children,
}: {
  title: string;
  onAdd: () => void;
  empty: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">{title}</h2>
        <Button size="sm" variant="outline" className="ml-auto h-7" onClick={onAdd}>
          <Plus className="size-4" aria-hidden />
          Novo
        </Button>
      </div>

      {empty || (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(280px,1fr))]">
          {children}
        </div>
      )}
    </section>
  );
}
