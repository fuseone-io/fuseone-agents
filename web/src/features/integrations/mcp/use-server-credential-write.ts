import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { isStale } from "@/lib/api/revision";
import type { InOrder } from "@/features/integrations/mcp/use-ordered-writes";
import { serverProblemMessage } from "@/features/integrations/mcp/server-refusal";
import {
  usePutMCPServer,
  type MCPGitHubApp,
  type MCPOAuthGrant,
  type MCPServer,
} from "@/features/integrations/api";

export type ServerCredentialWrite = {
  token?: string;
  headers?: Record<string, string>;
  oauth?: MCPOAuthGrant;
  githubApp?: MCPGitHubApp;
  env?: Record<string, string>;
  configFile?: string;
  configFileEnv?: string;
};

/** A server's credential write, and whether one is under way. */
export type ServerConnection = {
  write: (credential: ServerCredentialWrite) => Promise<boolean>;
  pending: boolean;
};

/** How its page orders writes to the server, and what to do when one is refused as stale. */
export type ServerRevision = { inOrder: InOrder; onStale: () => void };

/**
 * One write of a server's credential, with everything else sent back as read.
 *
 * Passed through exactly as given. An undefined token means this write is not
 * about the token, and an empty one means somebody is removing it — collapsing
 * the two is how a revoke button stops revoking.
 */
export function useServerCredentialWrite(server: MCPServer, revision?: ServerRevision): ServerConnection {
  const { t } = useTranslation();
  const put = usePutMCPServer();

  async function write(credential: ServerCredentialWrite): Promise<boolean> {
    const send = (record: MCPServer, etag?: string) =>
      put.mutateAsync({
        name: record.name,
        // Where it is stored, as read: a server's scope does not move.
        scope: record,
        transport: record.transport ?? "stdio",
        protocolMode: record.protocolMode ?? "auto",
        command: record.command ?? "",
        args: record.args ?? [],
        url: record.url ?? "",
        enabled: record.enabled,
        acceptsLocalExecution: record.acceptsLocalExecution ?? false,
        token: credential.token,
        headers: credential.headers,
        oauth: credential.oauth,
        githubApp: credential.githubApp,
        env: credential.env,
        configFile: credential.configFile,
        configFileEnv: credential.configFileEnv,
        etag,
      });
    try {
      // On the server's page, after any write before it and with the record
      // stored then; alone, with the server it was given and no revision.
      await (revision ? revision.inOrder(({ body, etag }) => send(body, etag)) : send(server));
      toast.success(t("mcp.credentialSaved"));
      return true;
    } catch (problem) {
      if (revision && isStale(problem)) revision.onStale();
      else toast.error(serverProblemMessage(problem, t));
      return false;
    }
  }

  return { write, pending: put.isPending };
}
