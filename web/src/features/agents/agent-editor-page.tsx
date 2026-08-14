import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { EditorBody } from "@/features/agents/editor-body";
import { EditorFooter } from "@/features/agents/editor-footer";
import { EditorTabBar } from "@/features/agents/editor-tab-bar";
import { EditorHeader } from "@/features/agents/editor-header";
import { counts, type EditorTab } from "@/features/agents/editor-tabs";
import { useAgentDraft } from "@/features/agents/agent-draft";
import { usePublishAgent } from "@/features/agents/agent-editor-api";
import { useAgent } from "@/features/agents/agent-detail-api";
import { useTools } from "@/features/admin/api";
import { usePolicies } from "@/features/policies/api";

/**
 * One agent, written or rewritten.
 *
 * Four tabs rather than one column. The column held six unrelated decisions
 * and about fifty controls, with the primary action below all of them —
 * reachable only after scrolling past everything it was about to publish.
 *
 * Two modes, one screen, because they are one act: the identifier is the
 * agent, and everything else is what it says this time. The button names the
 * version it writes, because "save" would hide that editing an agent creates
 * something a run will be pinned to.
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
  const [tab, setTab] = useState<EditorTab>("definition");
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

  const ready =
    agentId !== "" && draft.name !== "" && draft.instructions.trim() !== "";

  const submit = () =>
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
          );
          void navigate(`/agents/${agentId}`);
        },
        onError: () => toast.error(t("agents.publishFailed")),
      },
    );

  const catalogue = tools.data?.items ?? [];
  const rules = policies.data?.items ?? [];

  return (
    // Fills what the shell gives it, which is everything under the header
    // once this screen has asked for compact chrome. Computing a height from
    // the viewport instead was wrong by the padding it could not see.
    <div className="flex h-full min-h-0 flex-col">
      <EditorHeader
        agentId={agentId}
        name={draft.name || t(creating ? "agents.newAgent" : "agents.untitled")}
        version={loaded.data?.agent.versionId}
        stage={t(`stage.${loaded.data?.agent.stage ?? "draft"}`)}
      />
      <EditorTabBar active={tab} onChange={setTab} counts={counts(draft)} />

      {/* Unpadded, so a tab with a bar of its own can reach both edges, and
          not scrolling: whichever tab is open owns its own scrolling. Two
          scroll containers in one column is how the last row of a filling tab
          ends up under the footer — it was scrolled by the outer one, which
          had no idea the footer was there. */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <EditorBody
            tab={tab}
            draft={draft}
            patch={patch}
            editing={{
              agentId,
              creating,
              onAgentId: setAgentId,
              template: fromTemplate,
              onTemplate: setFromTemplate,
            }}
            tools={{ catalogue, policies: rules }}
          />
      </div>

      <EditorFooter
        changes={changes}
        creating={creating}
        publishing={publish.isPending}
        ready={ready}
        onPublish={submit}
        onDiscard={() => void navigate("/agents")}
      />
    </div>
  );
}
