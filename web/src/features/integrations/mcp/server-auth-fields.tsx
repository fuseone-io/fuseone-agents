import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { AuthChoicePicker } from "@/features/integrations/mcp/auth-choice-picker";
import { remoteTokenHint, remoteTokenLabel } from "@/features/integrations/mcp/credential-labels";
import { CustomHeaderFormFields } from "@/features/integrations/mcp/custom-header-form-fields";
import { GitHubAppFormFields } from "@/features/integrations/mcp/github-app-form-fields";
import { OAuthGrantFields } from "@/features/integrations/mcp/oauth-grant-fields";
import { RemoteAuthSummary } from "@/features/integrations/mcp/connection-summary";
import { ServerHeaderFields } from "@/features/integrations/mcp/server-header-fields";
import { clearShapes, formChoice, serverFormAuth } from "@/features/integrations/mcp/server-form-auth";

/**
 * How a remote server authenticates, inside the connect and edit form: the
 * shape first, then only its fields.
 *
 * The form opens on the stored shape and follows a recipe that arrives after
 * it. A change of shape clears every shape's fields, and a save sends only
 * the one shown (formCredential).
 */
export function ServerAuthFields({
  form,
  server,
  recipe,
}: {
  form: UseFormReturn<ServerFormValues>;
  server?: MCPServer | null;
  recipe?: ServerRecipe | null;
}) {
  const { t } = useTranslation();
  const auth = serverFormAuth(recipe, server);
  const choice = formChoice(auth, form.watch("authChoice"));
  const kept = server?.hasSecret === true;
  /*
   * Whenever the shape shown changes, every shape's fields start empty: on a
   * pick, and as much when a recipe arriving after the form changes it with
   * nobody touching anything. Left behind, a value typed under the old shape
   * would still be validated, and an invalid one nobody can see any more
   * would refuse every save.
   */
  const shown = useRef(choice);
  useEffect(() => {
    if (shown.current === choice) return;
    shown.current = choice;
    clearShapes(form);
  }, [choice, form]);
  return (
    <>
      <RemoteAuthSummary plan={auth.plan} />
      <AuthChoicePicker
        id="form-auth"
        choices={auth.choices}
        value={choice}
        onChange={(next) => form.setValue("authChoice", next, { shouldDirty: true })}
      />
      {choice === "token" && (
        <FormField
          control={form.control}
          name="token"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{remoteTokenLabel(auth.plan.secret, t)}</FormLabel>
              <FormControl>
                <Input {...field} type="password" autoComplete="off" className="font-mono" />
              </FormControl>
              <FormDescription>
                {kept ? t("integrations.tokenKept") : remoteTokenHint(auth.plan.secret, t)}
              </FormDescription>
            </FormItem>
          )}
        />
      )}
      {choice === "headers" && <ServerHeaderFields form={form} headers={auth.headers} kept={kept} />}
      {choice === "customHeader" && <CustomHeaderFormFields form={form} kept={kept} />}
      {choice === "oauth" && (
        <div className="space-y-3 rounded-lg border p-3">
          <p className="text-xs text-muted-foreground">
            {server?.hasOAuth ? t("mcp.oauthKept") : t("mcp.oauthHint")}
          </p>
          <OAuthGrantFields
            value={form.watch()}
            onChange={(field, next) => form.setValue(field, next, { shouldDirty: true, shouldValidate: true })}
          />
          <OAuthErrors form={form} />
        </div>
      )}
      {choice === "githubApp" && <GitHubAppFormFields form={form} />}
    </>
  );
}

// The grant's refusals: an expiry that is not Unix seconds, and a grant with no token.
function OAuthErrors({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  const { errors } = form.formState;
  const messages = [errors.oauthExpiresAtUnix?.message, errors.oauthAccessToken?.message].filter(
    (message): message is string => Boolean(message),
  );
  return messages.map((message) => (
    <p key={message} className="text-xs text-danger">
      {t(message)}
    </p>
  ));
}
