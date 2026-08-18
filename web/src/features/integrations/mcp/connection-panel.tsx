import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Mono } from "@/components/shared/mono";
import { problemMessage } from "@/lib/api/problem-message";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import {
  remoteAuthPlan,
  type AuthMode,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import {
  oauthExpiryIsValid,
  oauthFromValue,
  oauthHasValue,
} from "@/features/integrations/mcp/oauth-credential";
import { readVariables } from "@/features/integrations/mcp/variables";
import {
  usePutMCPServer,
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
  const local = (server.transport ?? "stdio") === "stdio";
  const remotePlan = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
  const storedConfigEnv = server.configFileEnv ?? "";
  const [value, setValue] = useState(() => blankCredential(storedConfigEnv));
  const oauthChanged = oauthHasValue(value);
  const bearerChanged = remotePlan.bearer !== null && value.token !== "";
  const remoteConflict =
    !local && remotePlan.oauth !== null && oauthChanged && bearerChanged;
  const oauthExpiryInvalid =
    !local && remotePlan.oauth !== null && !oauthExpiryIsValid(value);
  const canWriteRemote = remotePlan.bearer !== null || remotePlan.oauth !== null;

  async function write(credential: {
    token?: string;
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
        command: server.command ?? "",
        args: server.args ?? [],
        url: server.url ?? "",
        enabled: server.enabled,
        acceptsLocalExecution: server.acceptsLocalExecution ?? false,
        token: credential.token,
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

        <CredentialFields
          local={local}
          hasSecret={local ? server.hasVariables === true : server.hasSecret === true && server.hasOAuth !== true}
          hasConfigFile={server.hasConfigFile === true}
          showRemoteToken={remotePlan.bearer !== null}
          remoteTokenLabel={remoteTokenLabel(remotePlan.bearer, t)}
          remoteTokenHint={remoteTokenHint(remotePlan.bearer, t)}
          value={value}
          onChange={(next) => setValue((current) => ({ ...current, ...next }))}
          onRevoke={() =>
            // Explicit, and empty rather than absent: the two are different
            // requests and only one of them is somebody revoking.
            void write(local ? { env: {} } : { token: "" })
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
        {!local && remotePlan.oauth === null && server.hasOAuth === true && (
          <StoredOAuthOnly onRevoke={() => void write({ oauth: {} })} />
        )}

        <div className="flex justify-end">
          <Button
            onClick={() =>
              void write(
                local
                  ? {
                      ...(value.env === "" ? {} : { env: readVariables(value.env) }),
                      ...(value.configFile === ""
                        ? {}
                        : { configFile: value.configFile }),
                      configFileEnv: value.configFileEnv,
                    }
                  : remotePlan.oauth !== null && oauthChanged
                    ? { oauth: oauthFromValue(value) }
                    : { token: value.token },
              )
            }
            disabled={
              put.isPending ||
              remoteConflict ||
              oauthExpiryInvalid ||
              (local
                ? value.env === "" &&
                  value.configFile === "" &&
                  value.configFileEnv === storedConfigEnv
                : !canWriteRemote ||
                  ((remotePlan.bearer === null || value.token === "") &&
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

type CredentialValue = {
  token: string;
  env: string;
  configFile: string;
  configFileEnv: string;
  oauthAccessToken: string;
  oauthRefreshToken: string;
  oauthTokenURL: string;
  oauthClientID: string;
  oauthClientSecret: string;
  oauthTokenType: string;
  oauthExpiresAtUnix: string;
  oauthScopes: string;
};

function blankCredential(configFileEnv: string): CredentialValue {
  return {
    token: "",
    env: "",
    configFile: "",
    configFileEnv,
    oauthAccessToken: "",
    oauthRefreshToken: "",
    oauthTokenURL: "",
    oauthClientID: "",
    oauthClientSecret: "",
    oauthTokenType: "",
    oauthExpiresAtUnix: "",
    oauthScopes: "",
  };
}

function remoteTokenLabel(
  mode: AuthMode | null,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return mode?.label ?? t("mcp.remoteBearerToken");
}

function remoteTokenHint(
  mode: AuthMode | null,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!mode) return undefined;
  const header = mode.header ?? "Authorization";
  const prefix = mode.prefix ?? "Bearer";
  return t("mcp.remoteBearerHint", { header, prefix });
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

  if (plan.noAuth !== null && plan.bearer === null && plan.oauth === null) {
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

function OAuthFields({
  value,
  hasOAuth,
  conflict,
  invalidExpiry,
  onChange,
  onRevoke,
}: {
  value: CredentialValue;
  hasOAuth: boolean;
  conflict: boolean;
  invalidExpiry: boolean;
  onChange: (next: CredentialValue) => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 border-t pt-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <OAuthInput
          id="oauth-access-token"
          label={t("mcp.oauthAccessToken")}
          type="password"
          value={value.oauthAccessToken}
          onChange={(oauthAccessToken) => onChange({ ...value, oauthAccessToken })}
        />
        <OAuthInput
          id="oauth-refresh-token"
          label={t("mcp.oauthRefreshToken")}
          type="password"
          value={value.oauthRefreshToken}
          onChange={(oauthRefreshToken) => onChange({ ...value, oauthRefreshToken })}
        />
        <OAuthInput
          id="oauth-token-url"
          label={t("mcp.oauthTokenURL")}
          value={value.oauthTokenURL}
          onChange={(oauthTokenURL) => onChange({ ...value, oauthTokenURL })}
        />
        <OAuthInput
          id="oauth-client-id"
          label={t("mcp.oauthClientID")}
          value={value.oauthClientID}
          onChange={(oauthClientID) => onChange({ ...value, oauthClientID })}
        />
        <OAuthInput
          id="oauth-client-secret"
          label={t("mcp.oauthClientSecret")}
          type="password"
          value={value.oauthClientSecret}
          onChange={(oauthClientSecret) => onChange({ ...value, oauthClientSecret })}
        />
        <OAuthInput
          id="oauth-token-type"
          label={t("mcp.oauthTokenType")}
          placeholder={t("mcp.oauthTokenTypePlaceholder")}
          value={value.oauthTokenType}
          onChange={(oauthTokenType) => onChange({ ...value, oauthTokenType })}
        />
        <OAuthInput
          id="oauth-expires-at"
          label={t("mcp.oauthExpiresAtUnix")}
          value={value.oauthExpiresAtUnix}
          onChange={(oauthExpiresAtUnix) => onChange({ ...value, oauthExpiresAtUnix })}
        />
        <div className="space-y-1.5">
          <Label htmlFor="oauth-scopes">{t("mcp.oauthScopes")}</Label>
          <Textarea
            id="oauth-scopes"
            rows={2}
            className="font-mono text-xs"
            value={value.oauthScopes}
            onChange={(e) => onChange({ ...value, oauthScopes: e.target.value })}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {hasOAuth ? t("mcp.oauthKept") : t("mcp.oauthHint")}
      </p>
      {conflict && (
        <p className="text-xs text-danger">{t("mcp.oauthBearerConflict")}</p>
      )}
      {invalidExpiry && (
        <p className="text-xs text-danger">{t("mcp.oauthExpiryInvalid")}</p>
      )}
      {hasOAuth && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRevoke}
          className="text-danger"
        >
          <Trash2 className="size-3.5" />
          {t("mcp.revokeOAuth")}
        </Button>
      )}
    </div>
  );
}

function StoredOAuthOnly({ onRevoke }: { onRevoke: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 border-t pt-3">
      <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
        {t("mcp.storedOAuthOutsideRecipe")}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRevoke}
        className="text-danger"
      >
        <Trash2 className="size-3.5" />
        {t("mcp.revokeOAuth")}
      </Button>
    </div>
  );
}

function OAuthInput({
  id,
  label,
  type = "text",
  placeholder,
  value,
  onChange,
}: {
  id: string;
  label: string;
  type?: string;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        autoComplete="off"
        className="font-mono"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
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
