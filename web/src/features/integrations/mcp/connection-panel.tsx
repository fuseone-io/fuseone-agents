import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { problemMessage } from "@/lib/api/problem-message";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import {
  dsnEnvMode,
  headerNames,
  remoteAuthPlan,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import {
  blankCredential,
  localCredential,
  remoteCredential,
} from "@/features/integrations/mcp/credential-value";
import {
  oauthExpiryIsValid,
  oauthFromValue,
  oauthHasValue,
} from "@/features/integrations/mcp/oauth-credential";
import {
  OAuthFields,
  StoredOAuthOnly,
} from "@/features/integrations/mcp/oauth-fields";
import { CallAuthNote } from "@/features/integrations/mcp/call-auth-note";
import { EgressNote } from "@/features/integrations/mcp/egress-note";
import {
  remoteTokenHint,
  remoteTokenLabel,
} from "@/features/integrations/mcp/credential-labels";
import {
  usePutMCPServer,
  useProbeMCPServer,
  type MCPOAuthGrant,
  type MCPServer,
} from "@/features/integrations/api";

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
}: {
  server: MCPServer;
  recipe?: ServerRecipe | null;
}) {
  const { t } = useTranslation();
  const put = usePutMCPServer();
  const probe = useProbeMCPServer();
  const local = (server.transport ?? "stdio") === "stdio";
  const remotePlan = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
  const dsnMode = local ? dsnEnvMode(recipe?.authModes) : null;
  const remoteHeaders = headerNames(remotePlan.multiHeaders);
  const storedConfigEnv = server.configFileEnv ?? "";
  const [value, setValue] = useState(() => blankCredential(storedConfigEnv));
  const oauthChanged = oauthHasValue(value);
  const secretChanged = remotePlan.secret !== null && value.token !== "";
  const headersChanged = remoteHeaders.some((header) => value.headers[header] !== "");
  const headersComplete =
    remoteHeaders.length > 0 &&
    remoteHeaders.every((header) => value.headers[header]?.trim() !== "");
  const dsnChanged = dsnMode !== null && value.dsn !== "";
  const remoteSecretConflict = !local && secretChanged && headersChanged;
  const remoteConflict =
    !local &&
    remotePlan.oauth !== null &&
    oauthChanged &&
    (secretChanged || headersChanged);
  const oauthExpiryInvalid =
    !local && remotePlan.oauth !== null && !oauthExpiryIsValid(value);
  const canWriteRemote =
    remotePlan.secret !== null || remoteHeaders.length > 0 || remotePlan.oauth !== null;

  async function write(credential: {
    token?: string;
    headers?: Record<string, string>;
    oauth?: MCPOAuthGrant;
    env?: Record<string, string>;
    configFile?: string;
    configFileEnv?: string;
  }) {
    // Passed through exactly as given. An undefined token means this write is
    // not about the token, and an empty one means somebody is removing it —
    // collapsing the two is how a revoke button stops revoking.
    try {
      await put.mutateAsync({
        name: server.name,
        transport: server.transport ?? "stdio",
        protocolMode: server.protocolMode ?? "auto",
        command: server.command ?? "",
        args: server.args ?? [],
        url: server.url ?? "",
        enabled: server.enabled,
        acceptsLocalExecution: server.acceptsLocalExecution ?? false,
        token: credential.token,
        headers: credential.headers,
        oauth: credential.oauth,
        env: credential.env,
        configFile: credential.configFile,
        configFileEnv: credential.configFileEnv,
      });
      setValue(blankCredential(credential.configFileEnv ?? storedConfigEnv));
      toast.success(t("mcp.credentialSaved"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

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
          <Row label={t("integrations.transport")} value={server.transport ?? "stdio"} />
          {local ? (
            <Row label={t("integrations.command")} value={[server.command, ...(server.args ?? [])].join(" ")} />
          ) : (
            <Row label={t("integrations.url")} value={server.url ?? ""} />
          )}
        </dl>

        {!local && <RemoteAuthSummary plan={remotePlan} />}
        <CallAuthNote callAuth={server.callAuth} />
        <EgressNote egress={server.egress} />

        <CredentialFields
          local={local}
          hasSecret={local ? server.hasVariables === true : server.hasSecret === true && server.hasOAuth !== true}
          hasConfigFile={server.hasConfigFile === true}
          showRemoteToken={remotePlan.secret !== null}
          remoteTokenLabel={remoteTokenLabel(remotePlan.secret, t)}
          remoteTokenHint={remoteTokenHint(remotePlan.secret, t)}
          remoteHeaders={remoteHeaders}
          remoteHeadersHint={t("mcp.remoteHeadersHint")}
          dsnLabel={dsnMode ? (dsnMode.label ?? t("mcp.dsn")) : undefined}
          dsnHint={
            dsnMode
              ? t("mcp.dsnHint", { env: dsnMode.env ?? "DATABASE_URL" })
              : undefined
          }
          value={value}
          onChange={(next) => setValue((current) => ({ ...current, ...next }))}
          onRevoke={() =>
            // Explicit, and empty rather than absent: the two are different
            // requests and only one of them is somebody revoking.
            void write(local ? { env: {} } : { token: "", headers: {} })
          }
          onRevokeConfigFile={() => void write({ configFile: "" })}
        />
        {!local && remotePlan.oauth !== null && (
          <OAuthFields
            value={value}
            hasOAuth={server.hasOAuth === true}
            conflict={remoteConflict}
            invalidExpiry={oauthExpiryInvalid}
            onChange={setValue}
            onRevoke={() => void write({ oauth: {} })}
          />
        )}
        {remoteSecretConflict && (
          <p className="text-xs text-danger">{t("mcp.remoteCredentialConflict")}</p>
        )}
        {!local && remotePlan.oauth === null && server.hasOAuth === true && (
          <StoredOAuthOnly onRevoke={() => void write({ oauth: {} })} />
        )}

        <div className="flex flex-wrap items-center justify-between gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => void probeNow()}
            disabled={probe.isPending || !server.enabled}
          >
            <RefreshCw className="size-3.5" />
            {t("mcp.probe")}
          </Button>
          <Button
            onClick={() =>
              void write(
                local
                  ? {
                      ...localCredential(value, dsnMode),
                      ...(value.configFile === ""
                        ? {}
                        : { configFile: value.configFile }),
                      configFileEnv: value.configFileEnv,
                    }
                  : remotePlan.oauth !== null && oauthChanged
                    ? { oauth: oauthFromValue(value) }
                    : remoteCredential(value, remotePlan, remoteHeaders),
              )
            }
            disabled={
              put.isPending ||
              remoteSecretConflict ||
              remoteConflict ||
              oauthExpiryInvalid ||
              (local
                ? value.env === "" &&
                  !dsnChanged &&
                  value.configFile === "" &&
                  value.configFileEnv === storedConfigEnv
                : !canWriteRemote ||
                  ((remotePlan.secret === null || value.token === "") &&
                    (remoteHeaders.length === 0 || !headersComplete) &&
                    (remotePlan.oauth === null || !oauthChanged)))
            }
          >
            {t("mcp.saveCredential")}
          </Button>
        </div>
      </div>
    </Panel>
  );
}

function RemoteAuthSummary({ plan }: { plan: RemoteAuthPlan }) {
  const { t } = useTranslation();
  if (!plan.known) {
    return (
      <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
        {t("mcp.authUnknownShape")}
      </p>
    );
  }

  if (plan.noAuth !== null && plan.secret === null && plan.oauth === null) {
    return (
      <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
        {t("mcp.authNoCredential")}
      </p>
    );
  }

  if (plan.unsupported.length === 0) return null;
  const modes = plan.unsupported
    .map((mode) => mode.label ?? t(`mcp.authMode.${mode.type}`))
    .join(", ");
  return (
    <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
      {t("mcp.authShapeUnsupported", { modes })}
    </p>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2">
      <dt className="w-28 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate">
        <Mono className="text-xs">{value}</Mono>
      </dd>
    </div>
  );
}
