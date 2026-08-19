import { useMemo, useState } from "react";
import { KeyRound, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Mono } from "@/components/shared/mono";
import { problemMessage } from "@/lib/api/problem-message";
import { cn } from "@/lib/utils";
import type {
  MCPOAuthGrant,
  MCPServer,
  MCPUserCredential,
} from "@/features/integrations/api";
import {
  useDeleteMCPUserCredential,
  usePutMCPUserCredential,
} from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { CatalogueIcon } from "@/features/integrations/mcp/catalogue-icons";
import {
  AuthModeBadges,
  RecipeStatusBadge,
} from "@/features/integrations/mcp/recipe-badges";
import {
  headerNames,
  remoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import {
  blankCredential,
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
import {
  remoteTokenHint,
  remoteTokenLabel,
} from "@/features/integrations/mcp/credential-labels";

export function UserCredentialsPanel({
  servers,
  recipes,
  credentials,
  isLoading,
  error,
  onRetry,
}: {
  servers: MCPServer[];
  recipes: ServerRecipe[];
  credentials: MCPUserCredential[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const [chosen, setChosen] = useState<string | null>(null);
  const recipesByServer = useMemo(
    () => new Map(recipes.map((recipe) => [recipe.server, recipe])),
    [recipes],
  );
  const credentialsByServer = useMemo(
    () => new Map(credentials.map((credential) => [credential.server, credential])),
    [credentials],
  );
  const remoteServers = useMemo(
    () =>
      servers
        .filter((server) => (server.transport ?? "stdio") === "http")
        .sort((a, b) => a.name.localeCompare(b.name)),
    [servers],
  );
  const localCount = servers.length - remoteServers.length;
  const selected =
    remoteServers.find((server) => server.name === chosen) ??
    remoteServers[0] ??
    null;

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={onRetry} />;
  if (remoteServers.length === 0) {
    return (
      <EmptyState
        icon={<KeyRound className="size-6" />}
        title={t("mcp.noPersonalCredentialServers")}
        hint={t("mcp.noPersonalCredentialServersHint")}
      />
    );
  }

  return (
    <div className="grid min-h-[560px] overflow-hidden rounded-lg border bg-background lg:grid-cols-[20rem_minmax(0,1fr)]">
      <aside className="min-h-0 border-b bg-card lg:border-r lg:border-b-0">
        <div className="border-b p-4">
          <h2 className="text-sm font-medium">{t("mcp.personalCredentials")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("mcp.personalCredentialsHint")}
          </p>
        </div>
        <div className="min-h-0 max-h-[560px] space-y-1 overflow-y-auto p-2">
          {remoteServers.map((server) => {
            const recipe = recipesByServer.get(server.name) ?? null;
            const credential = credentialsByServer.get(server.name) ?? null;
            const active = selected?.name === server.name;
            return (
              <button
                key={server.name}
                type="button"
                onClick={() => setChosen(server.name)}
                className={cn(
                  "flex w-full items-start gap-3 rounded-md border border-transparent p-2 text-left hover:bg-muted",
                  active && "border-border bg-muted shadow-xs",
                )}
              >
                <CatalogueIcon
                  entry={{
                    name: server.name,
                    category: recipe?.category ?? "operations",
                  }}
                  className="size-8 rounded-md"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {recipe?.title ?? server.name}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {credential
                      ? t("mcp.personalCredentialStored")
                      : t("mcp.personalCredentialMissing")}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
        {localCount > 0 && (
          <p className="border-t p-3 text-xs text-muted-foreground">
            {t("mcp.personalCredentialsLocalNote", { count: localCount })}
          </p>
        )}
      </aside>

      {selected && (
        <PersonalCredentialEditor
          key={selected.name}
          server={selected}
          recipe={recipesByServer.get(selected.name) ?? null}
          credential={credentialsByServer.get(selected.name) ?? null}
        />
      )}
    </div>
  );
}

function PersonalCredentialEditor({
  server,
  recipe,
  credential,
}: {
  server: MCPServer;
  recipe: ServerRecipe | null;
  credential: MCPUserCredential | null;
}) {
  const { t } = useTranslation();
  const put = usePutMCPUserCredential();
  const remove = useDeleteMCPUserCredential();
  const remotePlan = remoteAuthPlan(recipe?.authModes, recipe !== null);
  const remoteHeaders = headerNames(remotePlan.multiHeaders);
  const [value, setValue] = useState(() => blankCredential());
  const oauthChanged = oauthHasValue(value);
  const secretChanged = remotePlan.secret !== null && value.token.trim() !== "";
  const headersChanged = remoteHeaders.some(
    (header) => value.headers[header]?.trim() !== "",
  );
  const headersComplete =
    remoteHeaders.length > 0 &&
    remoteHeaders.every((header) => value.headers[header]?.trim() !== "");
  const remoteSecretConflict = secretChanged && headersChanged;
  const remoteConflict =
    remotePlan.oauth !== null &&
    oauthChanged &&
    (secretChanged || headersChanged);
  const oauthExpiryInvalid =
    remotePlan.oauth !== null && !oauthExpiryIsValid(value);
  const canWrite =
    remotePlan.secret !== null ||
    remoteHeaders.length > 0 ||
    remotePlan.oauth !== null;
  const changed = secretChanged || headersComplete || oauthChanged;
  const hasSharedOnly =
    (server.hasSecret || server.hasOAuth) &&
    !credential;

  async function save(
    input: { token?: string; headers?: Record<string, string>; oauth?: MCPOAuthGrant },
  ) {
    try {
      await put.mutateAsync({ name: server.name, ...input });
      setValue(blankCredential());
      toast.success(t("mcp.personalCredentialSaved"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  async function revoke() {
    try {
      await remove.mutateAsync(server.name);
      setValue(blankCredential());
      toast.success(t("mcp.personalCredentialRemoved"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  return (
    <section className="min-w-0 overflow-y-auto p-5">
      <div className="mb-5 flex min-w-0 items-start gap-3">
        <CatalogueIcon
          entry={{
            name: server.name,
            category: recipe?.category ?? "operations",
          }}
          className="size-10 rounded-md"
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-base font-semibold">
              {recipe?.title ?? server.name}
            </h2>
            {recipe?.status && <RecipeStatusBadge status={recipe.status} />}
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            <Mono>{server.name}</Mono>
            {server.url ? ` · ${server.url}` : ""}
          </p>
        </div>
      </div>

      <div className="mb-5 grid gap-3 md:grid-cols-2">
        <div className="rounded-lg border bg-card p-3">
          <p className="text-xs font-medium">{t("mcp.sharedCredential")}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {hasSharedOnly
              ? t("mcp.sharedCredentialFallback")
              : t("mcp.sharedCredentialHint")}
          </p>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <p className="text-xs font-medium">{t("mcp.personalCredential")}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {credential
              ? personalCredentialLabel(credential, t)
              : t("mcp.personalCredentialEmpty")}
          </p>
        </div>
      </div>

      {recipe && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <AuthModeBadges modes={recipe.authModes ?? []} />
        </div>
      )}

      <div className="space-y-4 rounded-lg border bg-card p-4">
        <div className="flex gap-3 rounded-lg border bg-muted p-3">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <p className="text-xs text-muted-foreground">
            {t("mcp.personalCredentialScope")}
          </p>
        </div>

        <CredentialFields
          local={false}
          hasSecret={credential?.hasCredential === true && credential.hasOAuth !== true}
          hasConfigFile={false}
          showRemoteToken={remotePlan.secret !== null}
          remoteTokenLabel={remoteTokenLabel(remotePlan.secret, t)}
          remoteTokenHint={remoteTokenHint(remotePlan.secret, t)}
          remoteHeaders={remoteHeaders}
          remoteHeadersHint={t("mcp.remoteHeadersHint")}
          value={value}
          onChange={(next) => setValue((current) => ({ ...current, ...next }))}
          onRevoke={() => void revoke()}
          onRevokeConfigFile={() => undefined}
        />
        {remoteHeaders.length > 0 && headersChanged && !headersComplete && (
          <p className="text-xs text-danger">{t("mcp.remoteHeadersIncomplete")}</p>
        )}
        {remotePlan.oauth !== null && (
          <OAuthFields
            value={value}
            hasOAuth={credential?.hasOAuth === true}
            conflict={remoteConflict}
            invalidExpiry={oauthExpiryInvalid}
            onChange={setValue}
            onRevoke={() => void revoke()}
          />
        )}
        {remoteSecretConflict && (
          <p className="text-xs text-danger">{t("mcp.remoteCredentialConflict")}</p>
        )}
        {remotePlan.oauth === null && credential?.hasOAuth === true && (
          <StoredOAuthOnly onRevoke={() => void revoke()} />
        )}

        <div className="flex justify-end">
          <Button
            onClick={() =>
              void save(
                remotePlan.oauth !== null && oauthChanged
                  ? { oauth: oauthFromValue(value) }
                  : remoteCredential(value, remotePlan, remoteHeaders),
              )
            }
            disabled={
              put.isPending ||
              remove.isPending ||
              remoteSecretConflict ||
              remoteConflict ||
              oauthExpiryInvalid ||
              (headersChanged && !headersComplete) ||
              !canWrite ||
              !changed
            }
          >
            {t("mcp.savePersonalCredential")}
          </Button>
        </div>
      </div>
    </section>
  );
}

function personalCredentialLabel(
  credential: MCPUserCredential,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (credential.hasOAuth) return t("mcp.personalCredentialOAuth");
  if (credential.hasHeaders) return t("mcp.personalCredentialHeaders");
  if (credential.hasCredential) return t("mcp.personalCredentialBearer");
  return t("mcp.personalCredentialEmpty");
}
