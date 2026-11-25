import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { problemMessage } from "@/lib/api/problem-message";
import {
  useDeleteMCPUserCredential,
  usePutMCPUserCredential,
  type MCPUserCredential,
} from "@/features/integrations/api";
import type { CredentialServer } from "@/features/integrations/mcp/credential-server";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { headerNames, remoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import { authChoices, type AuthChoice } from "@/features/integrations/mcp/auth-choice";
import { secretShapeOffered, useAuthChoice } from "@/features/integrations/mcp/use-auth-choice";
import { StoredSecretOnly } from "@/features/integrations/mcp/stored-secret-only";
import { AuthChoicePicker } from "@/features/integrations/mcp/auth-choice-picker";
import { BrowserSignIn } from "@/features/integrations/mcp/browser-sign-in";
import { chosenCredential, chosenReady, type ChosenCredential } from "@/features/integrations/mcp/choice-credential";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";
import { blankCredential } from "@/features/integrations/mcp/credential-value";
import { oauthExpiryIsValid } from "@/features/integrations/mcp/oauth-credential";
import { OAuthFields, StoredOAuthOnly } from "@/features/integrations/mcp/oauth-fields";
import { remoteTokenHint, remoteTokenLabel } from "@/features/integrations/mcp/credential-labels";

/**
 * The signed-in person's own credential for one server, one shape at a time.
 *
 * Removing it removes the whole personal credential: a person holds one per
 * server, and the shared one is never touched from here.
 */
export function PersonalCredentialForm({
  server,
  recipe,
  credential,
}: {
  server: CredentialServer;
  recipe: ServerRecipe | null;
  credential: MCPUserCredential | null;
}) {
  const { t } = useTranslation();
  const put = usePutMCPUserCredential();
  const remove = useDeleteMCPUserCredential();
  const plan = remoteAuthPlan(recipe?.authModes, recipe !== null);
  const headers = headerNames(plan.multiHeaders);
  const choices = authChoices(plan, { customHeader: false, githubApp: false });
  const [choice, setChoice] = useAuthChoice(choices, {
      oauth: credential?.hasOAuth === true,
      githubApp: false,
      headers: credential?.hasHeaders === true ? headers : [],
    }, plan);
  const [value, setValue] = useState(() => blankCredential());
  const storedSecret = credential?.hasCredential === true && credential.hasOAuth !== true;
  const pending = put.isPending || remove.isPending;
  const headersTyped = headers.some((header) => (value.headers[header] ?? "").trim() !== "");

  async function save(input: ChosenCredential) {
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

  function choose(next: AuthChoice) {
    setChoice(next);
    setValue(blankCredential());
  }

  return (
    <>
      {server.signIn && <BrowserSignIn server={server.name} />}
      <AuthChoicePicker id="personal-auth" choices={choices} value={choice} onChange={choose} />
      {(choice === "token" || choice === "headers" || (choice === null && !storedSecret)) && (
        <CredentialFields
          local={false}
          hasSecret={storedSecret}
          hasConfigFile={false}
          showRemoteToken={choice === "token"}
          remoteTokenLabel={remoteTokenLabel(plan.secret, t)}
          remoteTokenHint={remoteTokenHint(plan.secret, t)}
          remoteHeaders={choice === "headers" ? headers : []}
          remoteHeadersHint={t("mcp.remoteHeadersHint")}
          value={value}
          onChange={(next) => setValue((current) => ({ ...current, ...next }))}
          onRevoke={() => void revoke()}
          onRevokeConfigFile={() => undefined}
        />
      )}
      {choice === "headers" && headersTyped && !chosenReady(choice, value, headers) && (
        <p className="text-xs text-danger">{t("mcp.remoteHeadersIncomplete")}</p>
      )}
      {choice === "oauth" && (
        <OAuthFields
          value={value}
          stored={{ hasOAuth: credential?.hasOAuth === true }}
          invalidExpiry={!oauthExpiryIsValid(value)}
          onChange={setValue}
          onRevoke={() => void revoke()}
        />
      )}
      {storedSecret && !secretShapeOffered(choices) && (
        <StoredSecretOnly onRevoke={() => void revoke()} />
      )}
      {plan.oauth === null && credential?.hasOAuth === true && (
        <StoredOAuthOnly onRevoke={() => void revoke()} />
      )}
      {choice !== null && (
        <div className="flex justify-end">
          <Button
            onClick={() => void save(chosenCredential(choice, value, plan, headers))}
            disabled={pending || !chosenReady(choice, value, headers)}
          >
            {t("mcp.savePersonalCredential")}
          </Button>
        </div>
      )}
    </>
  );
}
