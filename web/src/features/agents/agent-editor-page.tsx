import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { AgentEditorForm } from "@/features/agents/agent-editor-form";
import { AgentEditorRail } from "@/features/agents/agent-editor-rail";
import { useAgentDraft } from "@/features/agents/agent-draft";
import { TemplateGallery } from "@/features/agents/template-gallery";
import { usePublishAgent } from "@/features/agents/agent-editor-api";
import { useAgent } from "@/features/agents/agent-detail-api";
import { useTools } from "@/features/admin/api";
import { usePolicies } from "@/features/policies/api";

/**
 * One agent, written or rewritten.
 *
 * Two modes, one screen, because they are one act: the identifier is the
 * agent, and everything else is what it says this time. The primary button
 * names the version it will write, because t("common.save") would hide that editing
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
  const [fromTemplate, setFromTemplate] = useState<string>();
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
              ? t("agents.versionPublished", {
                  version: result.versionId.slice(0, 9),
                })
              : t("agents.noChange"),
            {
              description: result.created
                ? result.paused
                  ? t("agents.pausedStartWhenReady")
                  : t("agents.appliesNextRun")
                : t("agents.textAlreadyPublished"),
            },
          );
          navigate(`/agents/${agentId}`);
        },
        onError: (e) =>
          toast.error(t("agents.publishFailed"), {
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
        title={
          creating
            ? t("agents.newAgent")
            : t("agents.editing", { agent: draft.name || routeId })
        }
        description={t("agents.publishWritesVersion")}
      />

      {/* Above the form rather than instead of it: starting from nothing is
          still legitimate, and a gallery that replaced the form would make an
          author choose a template in order to delete it (PRD FU-16). */}
      {creating && (
        <TemplateGallery
          chosen={fromTemplate}
          onChoose={(template) => {
            patch({
              name: template.name,
              area: template.area ?? draft.area,
              instructions: template.instructions,
              triggers: template.triggers,
              budget: template.budget ?? draft.budget,
            });
            setAgentId(template.id);
            setFromTemplate(template.id);
          }}
          onClear={() => {
            // Back to the blank form. Clearing the text without clearing the
            // choice would leave a card marked as chosen above a form that no
            // longer holds any of it.
            patch({ name: "", instructions: "", triggers: [] });
            setAgentId("");
            setFromTemplate(undefined);
          }}
        />
      )}

      <div className="grid gap-5 lg:grid-cols-[1fr_316px] lg:items-start">
        <AgentEditorForm
          draft={draft}
          patch={patch}
          agentId={agentId}
          creating={creating}
          onAgentId={setAgentId}
          catalogue={tools.data?.items ?? []}
          policies={policies.data?.items ?? []}
        />

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
            {t("agents.unpublishedChanges", { count: changes.length })}
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" onClick={() => navigate("/agents")}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} disabled={publish.isPending || !ready}>
            {creating ? t("agents.createPaused") : t("agents.publishVersion")}
          </Button>
        </div>
      </div>
    </>
  );
}
