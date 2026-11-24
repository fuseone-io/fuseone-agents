import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import { headerNames, remoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import { authChoices, initialChoice, type AuthChoice } from "@/features/integrations/mcp/auth-choice";
import { AuthChoicePicker } from "@/features/integrations/mcp/auth-choice-picker";
import { chosenCredential, chosenReady } from "@/features/integrations/mcp/choice-credential";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { blankCredential } from "@/features/integrations/mcp/credential-value";
import { oauthExpiryIsValid } from "@/features/integrations/mcp/oauth-credential";
import { OAuthFields, StoredOAuthOnly } from "@/features/integrations/mcp/oauth-fields";
import { GitHubAppSection } from "@/features/integrations/mcp/github-app-section";
import { CustomHeaderSection } from "@/features/integrations/mcp/custom-header-section";
import { remoteTokenHint, remoteTokenLabel } from "@/features/integrations/mcp/credential-labels";
import type {
  ServerConnection,
  ServerCredentialWrite,
} from "@/features/integrations/mcp/use-server-credential-write";
import type { MCPServer } from "@/features/integrations/api";

/**
 * The credential of a remote server, one shape at a time.
 *
 * Leaving a field empty means "keep what is stored", because correcting an
 * address must not demand re-entering a secret nobody has to hand; removing is
 * each shape's own button. Switching shape starts the new one empty, and a
 * save sends only the shape chosen.
 */
export function RemoteCredentialEditor({
  server,
  recipe,
  connection,
}: {
  server: MCPServer;
  recipe?: ServerRecipe | null;
  connection: ServerConnection;
}) {
  const { t } = useTranslation();
  const plan = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
  const headers = headerNames(plan.multiHeaders);
  // A custom header only where the recipe was read and there is none:
  // undefined is not knowing yet.
  const choices = authChoices(plan, { customHeader: recipe === null, githubApp: Boolean(server.githubApp) });
  const [choice, setChoice] = useState<AuthChoice | null>(() =>
    initialChoice(choices, {
      oauth: server.hasOAuth === true,
      githubApp: Boolean(server.githubApp),
      headers: server.headerNames ?? [],
    }, plan),
  );
  const [value, setValue] = useState(() => blankCredential());
  const kept = server.hasSecret === true && server.hasOAuth !== true && !server.githubApp;

  async function write(credential: ServerCredentialWrite) {
    const saved = await connection.write(credential);
    if (saved) setValue(blankCredential());
    return saved;
  }

  function choose(next: AuthChoice) {
    setChoice(next);
    setValue(blankCredential());
  }

  const typed = choice === "token" || choice === "headers" || choice === "oauth";
  return (
    <>
      <AuthChoicePicker id="server-auth" choices={choices} value={choice} onChange={choose} />
      {(choice === null || choice === "token" || choice === "headers") && (
        <CredentialFields
          local={false}
          hasSecret={kept}
          hasConfigFile={false}
          showRemoteToken={choice === "token"}
          remoteTokenLabel={remoteTokenLabel(plan.secret, t)}
          remoteTokenHint={remoteTokenHint(plan.secret, t)}
          remoteHeaders={choice === "headers" ? headers : []}
          remoteHeadersHint={t("mcp.remoteHeadersHint")}
          value={value}
          onChange={(next) => setValue((current) => ({ ...current, ...next }))}
          onRevoke={() => void write({ token: "", headers: {} })}
          onRevokeConfigFile={() => undefined}
        />
      )}
      {choice === "oauth" && (
        <OAuthFields
          value={value}
          stored={{ hasOAuth: server.hasOAuth === true, client: server.oauthClient }}
          invalidExpiry={!oauthExpiryIsValid(value)}
          onChange={setValue}
          onRevoke={() => void write({ oauth: {} })}
        />
      )}
      {choice === "customHeader" && (
        <CustomHeaderSection pending={connection.pending} kept={kept} write={write} />
      )}
      {choice === "githubApp" && (
        <GitHubAppSection server={server} pending={connection.pending} write={write} />
      )}
      {plan.oauth === null && server.hasOAuth === true && (
        <StoredOAuthOnly onRevoke={() => void write({ oauth: {} })} />
      )}
      {typed && (
        <div className="flex justify-end">
          <Button
            onClick={() => void write(chosenCredential(choice, value, plan, headers))}
            disabled={connection.pending || !chosenReady(choice, value, headers)}
          >
            {t("mcp.saveCredential")}
          </Button>
        </div>
      )}
    </>
  );
}
