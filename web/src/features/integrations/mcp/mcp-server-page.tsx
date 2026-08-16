import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { Server } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { problemMessage } from "@/lib/api/problem-message";
import { SurfacePicker } from "@/features/integrations/mcp/surface-picker";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import { ClassifyDialog } from "@/features/admin/classify-dialog";
import type { Tool } from "@/features/admin/api";
import { useSetSurface } from "@/features/integrations/mcp/api";
import { useMCPServer } from "@/features/integrations/mcp/use-mcp-server";

/**
 * One tool server, in the order the decisions are actually made.
 *
 * Connection, then what this installation brought in, then what each tool is
 * allowed to do. They are separate acts by design and the page keeps them
 * separate: configuring a server never widens what agents may do, choosing the
 * surface is about scope rather than permission, and only the Curator's ruling
 * says what a tool does to the world.
 *
 * Its own route rather than a dialog under Settings. This is a governance
 * flow — three decisions with different consequences — and a modal that
 * disappears takes the record of what was chosen with it.
 */
export function MCPServerPage() {
  const { t } = useTranslation();
  const { name } = useParams();
  const {
    server, tools, catalogue, chosen, dirty, toggle, reset,
    isLoading, error, refetch,
  } = useMCPServer(name);
  const save = useSetSurface();
  const [ruling, setRuling] = useState<Tool | null>(null);

  async function saveSurface() {
    if (!server) return;
    try {
      await save.mutateAsync({
        name: server.name,
        surface: [...chosen],
        transport: server.transport ?? "stdio",
        command: server.command ?? "",
        args: server.args ?? [],
        url: server.url ?? "",
        enabled: server.enabled,
        acceptsLocalExecution: server.acceptsLocalExecution ?? false,
      });
      reset();
      toast.success(t("mcp.surfaceSaved"), {
        description: t("integrations.toolsAppearHint"),
      });
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  if (isLoading) return <LoadingRows />;
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  if (!server) {
    return <ErrorState error={new Error(t("mcp.noSuchServer"))} onRetry={refetch} />;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Server}
        title={server.name}
        description={t("mcp.subtitle")}
      />

      <ConnectionPanel server={server} />

      <Panel title={t("mcp.surface")} action={<Counted chosen={chosen.size} of={tools.length} />}>
        <div className="space-y-3">
          <p className="text-xs text-muted-foreground">{t("mcp.surfaceIs")}</p>
          <SurfacePicker
            tools={tools}
            chosen={chosen}
            onToggle={toggle}
            onClassify={setRuling}
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={reset} disabled={!dirty}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => void saveSurface()} disabled={!dirty || save.isPending}>
              {t("mcp.saveSurface")}
            </Button>
          </div>
        </div>
      </Panel>

      {/* The whole catalogue travels with it, not this server's share: what
          undoes a tool may live on another server, and a list that stopped
          here would hide every compensator the platform actually has. */}
      <ClassifyDialog
        tool={ruling}
        tools={catalogue}
        onClose={() => setRuling(null)}
      />
    </div>
  );
}

function Counted({ chosen, of }: { chosen: number; of: number }) {
  const { t } = useTranslation();
  return (
    <span className="text-xs text-muted-foreground">
      {t("mcp.broughtIn", { chosen, of })}
    </span>
  );
}
