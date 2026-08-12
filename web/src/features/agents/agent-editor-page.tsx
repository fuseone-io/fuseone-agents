import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { AgentEditorRail } from "@/features/agents/agent-editor-rail";
import { useAgentDraft } from "@/features/agents/agent-draft";
import { usePublishAgent } from "@/features/agents/agent-editor-api";
import { useAgent } from "@/features/agents/agent-detail-api";
import { useTools } from "@/features/admin/api";
import { usePolicies } from "@/features/policies/api";

/**
 * One agent, written or rewritten.
 *
 * Two modes, one screen, because they are one act: the identifier is the
 * agent, and everything else is what it says this time. The primary button
 * names the version it will write, because "Salvar" would hide that editing
 * an agent creates something a run will be pinned to.
 */
export function AgentEditorPage() {
  const { t } = useTranslation();
  const { agentId: routeId } = useParams();
  const creating = routeId === undefined || routeId === "new";
  const navigate = useNavigate();

  const loaded = useAgent(creating ? "" : (routeId ?? ""), undefined);
  const tools = useTools();
  const policies = usePolicies();
  const [agentId, setAgentId] = useState(creating ? "" : (routeId ?? ""));
  const { draft, patch, changes } = useAgentDraft(
    creating ? undefined : loaded.data,
  );
  const publish = usePublishAgent();

  if (!creating && loaded.isLoading) return <LoadingRows rows={6} />;
  if (!creating && loaded.error) {
    return (
      <ErrorState error={loaded.error} onRetry={() => void loaded.refetch()} />
    );
  }

  const submit = () => {
    publish.mutate(
      { agentId, definition: draft },
      {
        onSuccess: (result) => {
          toast.success(
            result.created
              ? `Versão ${result.versionId.slice(0, 9)} publicada`
              : "Nada mudou",
            {
              description: result.created
                ? result.paused
                  ? "O agente está pausado. Inicie quando quiser que ele rode."
                  : "Vale a partir da próxima execução."
                : "Este texto já era a versão publicada.",
            },
          );
          navigate(`/agents/${agentId}`);
        },
        onError: (e) =>
          toast.error("Não foi possível publicar", {
            description: e instanceof Error ? e.message : undefined,
          }),
      },
    );
  };

  const ready =
    agentId !== "" && draft.name !== "" && draft.instructions.trim() !== "";

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.agents}
        title={creating ? "Novo agente" : `Editar ${draft.name || routeId}`}
        description="Publicar escreve uma versão nova. As execuções já feitas continuam presas à versão que rodou nelas."
      />

      <div className="grid gap-5 lg:grid-cols-[1fr_316px] lg:items-start">
        <div className="flex flex-col gap-4">
          <AgentBasicsSection
            draft={draft}
            patch={patch}
            agentId={agentId}
            editable={creating}
            onAgentId={setAgentId}
          />
          <AgentToolsSection
            granted={draft.tools ?? []}
            catalogue={tools.data?.items ?? []}
            policies={policies.data?.items ?? []}
            onChange={(list) => patch({ tools: list })}
          />
          <AgentBudgetSection draft={draft} patch={patch} />
        </div>

        <AgentEditorRail
          draft={draft}
          catalogue={tools.data?.items ?? []}
          creating={creating}
          changes={changes}
        />
      </div>

      <div className="sticky bottom-0 -mx-6 -mb-6 flex items-center gap-2 border-t border-border bg-card px-6 py-3 shadow-md">
        {changes.length > 0 && (
          <span className="text-xs text-warning">
            {changes.length} {changes.length === 1 ? "alteração" : "alterações"}{" "}
            sem publicar
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" onClick={() => navigate("/agents")}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} disabled={publish.isPending || !ready}>
            {creating ? "Criar agente pausado" : "Publicar versão"}
          </Button>
        </div>
      </div>
    </>
  );
}
