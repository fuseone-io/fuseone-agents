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
  dsnEnvMode,
  headerCredential,
  headerNames,
  multiHeaderCredential,
  remoteAuthPlan,
  type AuthMode,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
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
  recipe?: ServerRecipe | null,
) {
  const { t } = useTranslation();
  const put = usePutMCPServer();

  const form = useForm<ServerFormValues>({
    resolver: zodResolver(serverSchema),
    defaultValues: {
      name: server?.name ?? "",
      transport: server?.transport ?? "stdio",
      protocolMode: server?.protocolMode ?? recipe?.protocolMode ?? "auto",
      command: server?.command ?? "",
      args: (server?.args ?? []).join(" "),
      url: server?.url ?? "",
      token: "",
      headers: {},
      env: "",
      dsn: "",
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
      rateLimitPerSecond: server?.rateLimit?.ratePerSecond?.toString() ?? "",
      rateLimitBurst: server?.rateLimit?.burst?.toString() ?? "",
      // Never carried forward from the transport. A server nobody has accepted
      // must show as not accepted, or the box would tick itself on the screen
      // where the decision is supposed to be made.
      acceptsLocalExecution: server?.acceptsLocalExecution ?? false,
      enabled: server?.enabled ?? true,
    },
  });

  async function submit(values: ServerFormValues) {
    const remotePlan = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
    const remoteHeaders = headerNames(remotePlan.multiHeaders);
    const dsnMode = values.transport === "stdio" ? dsnEnvMode(recipe?.authModes) : null;
    const hasSomeHeaders = remoteHeaders.some(
      (header) => values.headers[header]?.trim() !== "",
    );
    const hasAllHeaders =
      remoteHeaders.length > 0 &&
      remoteHeaders.every((header) => values.headers[header]?.trim() !== "");
    if (hasSomeHeaders && !hasAllHeaders) {
      form.setError("headers", { message: "mcp.remoteHeadersIncomplete" });
      return;
    }
    try {
      await put.mutateAsync({
        name: values.name,
        transport: values.transport,
        protocolMode: values.protocolMode,
        command: values.command,
        args: values.args.split(/\s+/).filter(Boolean),
        url: values.url,
        // Left empty means "leave what is stored", which is this form's whole
        // reason for not demanding a secret to correct an address. Removing
        // one is a separate gesture, on the server's own page.
        ...remoteCredential(values, remotePlan, remoteHeaders),
        env: localEnv(values, dsnMode),
        oauth: oauthHasValue(values) ? oauthFromValue(values) : undefined,
        configFile: values.configFile || undefined,
        configFileEnv: values.configFileEnv,
        rateLimit: rateLimitFromValues(values),
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

function remoteCredential(
  values: ServerFormValues,
  plan: RemoteAuthPlan,
  headers: string[],
) {
  if (values.transport !== "http") return {};
  if (values.token !== "" && plan.secret?.type === "bearer") {
    return { token: values.token };
  }
  if (values.token !== "" && plan.secret) {
    return { headers: headerCredential(plan.secret, values.token) };
  }
  if (headers.length > 0 && headers.every((header) => values.headers[header]?.trim() !== "")) {
    return { headers: multiHeaderCredential(headers, values.headers) };
  }
  return { token: values.token || undefined };
}

function localEnv(values: ServerFormValues, dsnMode: AuthMode | null) {
  if (values.transport !== "stdio") return undefined;
  if (values.env === "" && (dsnMode === null || values.dsn === "")) {
    return undefined;
  }
  const env = values.env === "" ? {} : readEnvLines(values.env);
  if (dsnMode?.env && values.dsn !== "") {
    env[dsnMode.env] = values.dsn;
  }
  return env;
}

function rateLimitFromValues(values: ServerFormValues) {
  const rate = Number(values.rateLimitPerSecond.trim() || "0");
  const burst = Number.parseInt(values.rateLimitBurst.trim() || "0", 10);
  return { ratePerSecond: rate, burst };
}

function readEnvLines(raw: string) {
  const out: Record<string, string> = {};
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed === "") continue;
    const at = trimmed.indexOf("=");
    if (at <= 0) continue;
    out[trimmed.slice(0, at)] = trimmed.slice(at + 1);
  }
  return out;
}
