import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { problemMessage } from "@/lib/api/problem-message";
import { remoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { CallAuthNote } from "@/features/integrations/mcp/call-auth-note";
import { EgressNote } from "@/features/integrations/mcp/egress-note";
import { MCPHealthNote } from "@/features/integrations/mcp/health-note";
import { ConnectionRow, RemoteAuthSummary } from "@/features/integrations/mcp/connection-summary";
import { LocalCredentialEditor } from "@/features/integrations/mcp/local-credential-editor";
import { RemoteCredentialEditor } from "@/features/integrations/mcp/remote-credential-editor";
import { SignInConfigSection } from "@/features/integrations/mcp/sign-in-config-section";
import {
  useServerCredentialWrite,
  type ServerRevision,
} from "@/features/integrations/mcp/use-server-credential-write";
import { useProbeMCPServer, type MCPServer } from "@/features/integrations/api";

/**
 * How this server is reached, and what it is given to reach it with.
 *
 * The credential is the part with a gesture of its own. Leaving a field empty
 * means "keep what is stored", because correcting an address must not demand
 * re-entering a secret nobody has to hand — and with only that rule a
 * credential could be written and never taken back, which is the half that
 * matters on the day it leaks.
 */
export function ConnectionPanel({
  server,
  recipe,
  revision,
}: {
  server: MCPServer;
  recipe?: ServerRecipe | null;
  /** How its page orders writes to the server, and what to do when one is refused as stale. */
  revision?: ServerRevision;
}) {
  const { t } = useTranslation();
  const probe = useProbeMCPServer();
  const connection = useServerCredentialWrite(server, revision);
  const local = (server.transport ?? "stdio") === "stdio";

  async function probeNow() {
    try {
      await probe.mutateAsync(server.name);
      toast.success(t("mcp.probeRequested"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  return (
    <Panel title={t("mcp.connection")}>
      <div className="space-y-4">
        <dl className="grid gap-1 text-xs">
          <ConnectionRow label={t("integrations.transport")} value={server.transport ?? "stdio"} />
          {local ? (
            <ConnectionRow
              label={t("integrations.command")}
              value={[server.command, ...(server.args ?? [])].join(" ")}
            />
          ) : (
            <ConnectionRow label={t("integrations.url")} value={server.url ?? ""} />
          )}
        </dl>

        {!local && (
          <RemoteAuthSummary
            plan={remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null)}
          />
        )}
        <MCPHealthNote health={server.health} />
        <CallAuthNote callAuth={server.callAuth} />
        <EgressNote egress={server.egress} />

        {local ? (
          <LocalCredentialEditor server={server} recipe={recipe} connection={connection} />
        ) : (
          <RemoteCredentialEditor server={server} recipe={recipe} connection={connection} />
        )}
        {/* People sign in as themselves; a server acting as an App has nobody to sign in. */}
        {!local && !server.githubApp && (
          // Opened again on what is stored whenever that changes, so the form
          // never holds an older configuration than the one it would replace.
          <SignInConfigSection key={JSON.stringify(server.oauthLogin ?? null)} server={server} connection={connection} />
        )}

        <Button
          type="button"
          variant="outline"
          onClick={() => void probeNow()}
          disabled={probe.isPending || !server.enabled}
        >
          <RefreshCw className="size-3.5" />
          {t("mcp.probe")}
        </Button>
      </div>
    </Panel>
  );
}
