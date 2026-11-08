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
import { isStale } from "@/lib/api/revision";
import { ApiError } from "@/lib/api/client";
import { StaleNotice } from "@/components/shared/stale-notice";
import { useMCPServerRevision } from "@/features/integrations/api";
import { SurfacePicker } from "@/features/integrations/mcp/surface-picker";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import { RediscoveryPanel } from "@/features/integrations/mcp/rediscovery-panel";
import { ClassifyDialog } from "@/features/admin/classify-dialog";
import type { Tool } from "@/features/admin/api";
import { useSetSurface } from "@/features/integrations/mcp/api";
import { useMCPServer } from "@/features/integrations/mcp/use-mcp-server";
import { useOrderedWrites } from "@/features/integrations/mcp/use-ordered-writes";

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
  // The record both writes send back, and the surface shown, come from the
  // server read by name, with the revision they answer; the listing still says
  // how it is answering.
  const read = useMCPServerRevision(name);
  const {
    server: listed, tools, catalogue, chosen, dirty, toggle, reset,
    isLoading, error, refetch,
  } = useMCPServer(name, read.data?.body);
  const server = listed && read.data ? { ...listed, ...read.data.body } : undefined;
  const save = useSetSurface();
  // The revision the page opened on, kept until a reload: a re-read brought
  // by anything else must not become what an edit begun before it answers.
  const [opened, setOpened] = useState<string>();
  if (opened === undefined && read.data && read.isFetchedAfterMount) {
    setOpened(read.data.etag);
  }
  const { inOrder, rebase } = useOrderedWrites(name, opened);
  const [ruling, setRuling] = useState<Tool | null>(null);
  const [stale, setStale] = useState(false);
  // Counted, not keyed by the revision: what is typed in the panel survives a
  // write elsewhere on the page, and is replaced only by a reload asked for.
  const [reloads, setReloads] = useState(0);

  function reload() {
    setStale(false);
    reset();
    refetch();
    void read.refetch().then((again) => {
      rebase();
      setOpened(again.data?.etag);
      setReloads((n) => n + 1);
    });
  }

  async function saveSurface() {
    if (!server) return;
    try {
      await inOrder(({ body, etag }) =>
        save.mutateAsync({
          name: body.name,
          surface: [...chosen],
          transport: body.transport ?? "stdio",
          protocolMode: body.protocolMode ?? "auto",
          command: body.command ?? "",
          args: body.args ?? [],
          url: body.url ?? "",
          enabled: body.enabled,
          acceptsLocalExecution: body.acceptsLocalExecution ?? false,
          etag,
        }),
      );
      reset();
      toast.success(t("mcp.surfaceSaved"), {
        description: t("integrations.toolsAppearHint"),
      });
    } catch (problem) {
      if (isStale(problem)) setStale(true);
      else toast.error(problemMessage(problem, t));
    }
  }

  // Only a read made as the page opened seeds its writes, never a cached copy.
  if (isLoading || (name && !read.isFetchedAfterMount)) return <LoadingRows />;
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  // The read by name is the answer, before any cached copy of it. Gone is no
  // such server, whatever a listing still behind says; a detail that could not
  // be read is that; neither ever opens the copy the last opening cached.
  const gone = read.error instanceof ApiError && read.error.status === 404;
  if (read.error && !gone) {
    return <ErrorState error={read.error} onRetry={() => void read.refetch()} />;
  }
  if (gone || !server) {
    return (
      <ErrorState
        error={new Error(t("mcp.noSuchServer"))}
        onRetry={() => {
          refetch();
          void read.refetch();
        }}
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Server}
        title={server.name}
        description={t("mcp.subtitle")}
      />

      {stale && <StaleNotice onReload={reload} reloading={read.isFetching} />}

      <ConnectionPanel
        key={reloads}
        server={server}
        revision={{ inOrder, onStale: () => setStale(true) }}
      />

      <RediscoveryPanel server={server.name} enabled={server.enabled} />

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
