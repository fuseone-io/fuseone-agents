import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  serverSchema,
  type ServerFormValues,
} from "@/features/integrations/server-schema";
import { usePutMCPServer, type MCPServer } from "@/features/integrations/api";
import {
  oauthFromValue,
  oauthHasValue,
} from "@/features/integrations/mcp/oauth-credential";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The connection form, wherever it is shown.
 *
 * Two screens hold it — a dialog for correcting one that exists, and a page
 * for connecting one that does not — and they must not drift: the schema
 * refuses the same mistakes and the write means the same thing. Two copies of
 * "an empty token leaves the stored one" is one copy that eventually revokes.
 */
export function useServerForm(
  server: MCPServer | null,
  onDone: (name: string) => void,
) {
  const { t } = useTranslation();
  const put = usePutMCPServer();

  const form = useForm<ServerFormValues>({
    resolver: zodResolver(serverSchema),
    defaultValues: {
      name: server?.name ?? "",
      transport: server?.transport ?? "stdio",
      command: server?.command ?? "",
      args: (server?.args ?? []).join(" "),
      url: server?.url ?? "",
      token: "",
      oauthAccessToken: "",
      oauthRefreshToken: "",
      oauthTokenURL: "",
      oauthClientID: "",
      oauthClientSecret: "",
      oauthTokenType: "",
      oauthExpiresAtUnix: "",
      oauthScopes: "",
      configFile: "",
      configFileEnv: server?.configFileEnv ?? "",
      // Never carried forward from the transport. A server nobody has accepted
      // must show as not accepted, or the box would tick itself on the screen
      // where the decision is supposed to be made.
      acceptsLocalExecution: server?.acceptsLocalExecution ?? false,
      enabled: server?.enabled ?? true,
    },
  });

  async function submit(values: ServerFormValues) {
    try {
      await put.mutateAsync({
        name: values.name,
        transport: values.transport,
        command: values.command,
        args: values.args.split(/\s+/).filter(Boolean),
        url: values.url,
        // Left empty means "leave what is stored", which is this form's whole
        // reason for not demanding a secret to correct an address. Removing
        // one is a separate gesture, on the server's own page.
        token: values.token || undefined,
        oauth: oauthHasValue(values) ? oauthFromValue(values) : undefined,
        configFile: values.configFile || undefined,
        configFileEnv: values.configFileEnv,
        acceptsLocalExecution: values.acceptsLocalExecution,
        enabled: values.enabled,
      });
      toast.success(t("integrations.serverConfigured", { name: values.name }), {
        // A worker picks the change up on its next pass rather than at its
        // next restart, which is a wait with an end somebody can be told.
        description: t("integrations.toolsAppearHint"),
      });
      onDone(values.name);
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return { form, submit, saving: put.isPending };
}
