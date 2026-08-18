import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import {
  remoteAuthPlan,
  type AuthMode,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

/**
 * The half of the form that depends on how the server is reached.
 *
 * A local server is a command this installation runs; a remote one is an
 * address it calls. Showing both at once would invite filling both, and only
 * one of them is ever used.
 */
export function ServerFields({
  form,
  hasSecret,
  hasConfigFile,
  recipe,
}: {
  form: UseFormReturn<ServerFormValues>;
  hasSecret: boolean;
  hasConfigFile: boolean;
  recipe?: ServerRecipe | null;
}) {
  const { t } = useTranslation();

  if (form.watch("transport") === "http") {
    const auth = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
    return (
      <>
        <Field
          form={form}
          name="url"
          label={t("integrations.url")}
          placeholder="https://api.example.com/mcp/"
        />
        <RemoteAuthNotice plan={auth} />
        {auth.bearer !== null && (
          <Field
            form={form}
            name="token"
            label={auth.bearer.label ?? t("mcp.remoteBearerToken")}
            placeholder=""
            hint={
              hasSecret
                ? t("integrations.tokenKept")
                : remoteTokenHint(auth.bearer, t)
            }
          />
        )}
        {auth.oauth !== null && <OAuthFormFields form={form} />}
      </>
    );
  }

  return (
    <>
      <Field
        form={form}
        name="command"
        label={t("integrations.command")}
        placeholder="/usr/local/bin/crm-mcp"
        hint={t("integrations.commandRuns")}
      />
      <Field
        form={form}
        name="args"
        label={t("integrations.arguments")}
        placeholder="--config /etc/crm.yaml"
      />
      <Field
        form={form}
        name="configFileEnv"
        label={t("mcp.configFileEnv")}
        placeholder="FUSEONE_MCP_CONFIG_FILE"
        hint={t("mcp.configFileEnvHint")}
      />
      <TextAreaField
        form={form}
        name="configFile"
        label={t("mcp.configFile")}
        placeholder={t("mcp.configFileExample")}
        hint={
          hasConfigFile ? t("mcp.configFileKept") : t("mcp.configFileHint")
        }
      />
      <AcceptLocalExecution form={form} />
    </>
  );
}

function remoteTokenHint(
  mode: AuthMode,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const header = mode.header ?? "Authorization";
  const prefix = mode.prefix ?? "Bearer";
  return t("mcp.remoteBearerHint", { header, prefix });
}

function RemoteAuthNotice({ plan }: { plan: RemoteAuthPlan }) {
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

function OAuthFormFields({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p className="text-xs text-muted-foreground">{t("mcp.oauthHint")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          form={form}
          name="oauthAccessToken"
          label={t("mcp.oauthAccessToken")}
          placeholder=""
          password
        />
        <Field
          form={form}
          name="oauthRefreshToken"
          label={t("mcp.oauthRefreshToken")}
          placeholder=""
          password
        />
        <Field
          form={form}
          name="oauthTokenURL"
          label={t("mcp.oauthTokenURL")}
          placeholder={t("mcp.oauthTokenURLPlaceholder")}
        />
        <Field
          form={form}
          name="oauthClientID"
          label={t("mcp.oauthClientID")}
          placeholder=""
        />
        <Field
          form={form}
          name="oauthClientSecret"
          label={t("mcp.oauthClientSecret")}
          placeholder=""
          password
        />
        <Field
          form={form}
          name="oauthTokenType"
          label={t("mcp.oauthTokenType")}
          placeholder={t("mcp.oauthTokenTypePlaceholder")}
        />
        <Field
          form={form}
          name="oauthExpiresAtUnix"
          label={t("mcp.oauthExpiresAtUnix")}
          placeholder=""
        />
        <TextAreaField
          form={form}
          name="oauthScopes"
          label={t("mcp.oauthScopes")}
          placeholder={t("mcp.oauthScopesPlaceholder")}
        />
      </div>
    </div>
  );
}

/**
 * What a local server is, said where the decision is made.
 *
 * Not a warning under the heading. A local server is a program this
 * installation starts inside the worker: it runs as the worker, on its
 * filesystem, from inside its network, and the Gate decides what a tool may do
 * while deciding nothing about what a process may read.
 *
 * A checkbox does not stop an administrator who means it, and pretending
 * otherwise would be worse than having none. What it does is make the
 * difference between the two transports impossible to pass without reading,
 * and put a name against the answer.
 */
function AcceptLocalExecution({
  form,
}: {
  form: UseFormReturn<ServerFormValues>;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name="acceptsLocalExecution"
      render={({ field }) => (
        <FormItem className="rounded-lg border border-destructive/40 bg-destructive/5 p-3">
          <div className="flex items-start gap-3">
            <FormControl>
              <Checkbox
                checked={field.value}
                onCheckedChange={field.onChange}
                className="mt-0.5"
              />
            </FormControl>
            <div className="space-y-1">
              <FormLabel className="m-0">
                {t("integrations.acceptLocalExecution")}
              </FormLabel>
              <FormDescription>
                {t("integrations.acceptLocalExecutionWhy")}
              </FormDescription>
            </div>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function Field({
  form,
  name,
  label,
  placeholder,
  hint,
  password = false,
}: {
  form: UseFormReturn<ServerFormValues>;
  name:
    | "url"
    | "token"
    | "command"
    | "args"
    | "configFileEnv"
    | "oauthAccessToken"
    | "oauthRefreshToken"
    | "oauthTokenURL"
    | "oauthClientID"
    | "oauthClientSecret"
    | "oauthTokenType"
    | "oauthExpiresAtUnix";
  label: string;
  placeholder: string;
  hint?: string;
  password?: boolean;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              className="font-mono"
              placeholder={placeholder}
              autoComplete="off"
              type={password || name === "token" ? "password" : undefined}
            />
          </FormControl>
          {hint && <FormDescription>{hint}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function TextAreaField({
  form,
  name,
  label,
  placeholder,
  hint,
}: {
  form: UseFormReturn<ServerFormValues>;
  name: "configFile" | "oauthScopes";
  label: string;
  placeholder: string;
  hint?: string;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Textarea
              {...field}
              className="font-mono text-xs"
              placeholder={placeholder}
              rows={5}
              autoComplete="off"
            />
          </FormControl>
          {hint && <FormDescription>{hint}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
