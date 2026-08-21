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
} from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import {
  dsnEnvMode,
  headerNames,
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
    const headers = headerNames(auth.multiHeaders);
    return (
      <>
        <Field
          form={form}
          name="url"
          label={t("integrations.url")}
          placeholder="https://api.example.com/mcp/"
        />
        <RemoteAuthNotice plan={auth} />
        {auth.secret !== null && (
          <Field
            form={form}
            name="token"
            label={auth.secret.label ?? t("mcp.remoteBearerToken")}
            placeholder=""
            hint={
              hasSecret
                ? t("integrations.tokenKept")
                : remoteTokenHint(auth.secret, t)
            }
          />
        )}
        {headers.length > 0 && (
          <HeaderFields form={form} headers={headers} hasSecret={hasSecret} />
        )}
        {auth.oauth !== null && <OAuthFormFields form={form} />}
        <RateLimitFields form={form} />
      </>
    );
  }
  const dsnMode = dsnEnvMode(recipe?.authModes);

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
      {dsnMode && (
        <Field
          form={form}
          name="dsn"
          label={dsnMode.label ?? t("mcp.dsn")}
          placeholder={t("mcp.dsnPlaceholder")}
          hint={t("mcp.dsnHint", { env: dsnMode.env ?? "DATABASE_URL" })}
          password
        />
      )}
      <TextAreaField
        form={form}
        name="env"
        label={t("mcp.variables")}
        placeholder={t("mcp.variablesExample")}
        hint={hasSecret ? t("mcp.variablesKept") : t("mcp.variablesHint")}
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
      <RateLimitFields form={form} />
      <AcceptLocalExecution form={form} />
    </>
  );
}

function RateLimitFields({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          form={form}
          name="rateLimitPerSecond"
          label={t("mcp.rateLimitPerSecond")}
          placeholder="1"
        />
        <Field
          form={form}
          name="rateLimitBurst"
          label={t("mcp.rateLimitBurst")}
          placeholder="5"
        />
      </div>
      <p className="text-xs text-muted-foreground">{t("mcp.rateLimitHint")}</p>
    </div>
  );
}

function remoteTokenHint(
  mode: AuthMode,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const header = mode.header ?? "Authorization";
  const prefix = mode.prefix;
  if (!prefix) return t("mcp.remoteHeaderHint", { header });
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
  if (
    plan.noAuth !== null &&
    plan.secret === null &&
    plan.multiHeaders === null &&
    plan.oauth === null
  ) {
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

function HeaderFields({
  form,
  headers,
  hasSecret,
}: {
  form: UseFormReturn<ServerFormValues>;
  headers: string[];
  hasSecret: boolean;
}) {
  const { t } = useTranslation();
  const error = form.formState.errors.headers;
  const message = typeof error?.message === "string" ? error.message : undefined;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="grid gap-3 sm:grid-cols-2">
        {headers.map((header) => (
          <div key={header} className="min-w-0 space-y-1.5">
            <FormLabel htmlFor={headerInputID(header)}>{header}</FormLabel>
            <Input
              id={headerInputID(header)}
              type="password"
              autoComplete="off"
              className="font-mono"
              value={(form.watch("headers") ?? {})[header] ?? ""}
              onChange={(event) =>
                form.setValue(
                  "headers",
                  { ...(form.getValues("headers") ?? {}), [header]: event.target.value },
                  { shouldDirty: true, shouldValidate: true },
                )
              }
            />
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {hasSecret ? t("integrations.tokenKept") : t("mcp.remoteHeadersHint")}
      </p>
      {message && <p className="text-xs text-danger">{t(message)}</p>}
    </div>
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
      render={({ field, fieldState }) => (
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
          <TranslatedMessage message={fieldState.error?.message} />
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
    | "dsn"
    | "command"
    | "args"
    | "configFileEnv"
    | "rateLimitPerSecond"
    | "rateLimitBurst"
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
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field, fieldState }) => (
        <FormItem className="min-w-0">
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
          <TranslatedMessage message={fieldState.error?.message} t={t} />
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
  name: "configFile" | "oauthScopes" | "env";
  label: string;
  placeholder: string;
  hint?: string;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field, fieldState }) => (
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
          <TranslatedMessage message={fieldState.error?.message} t={t} />
        </FormItem>
      )}
    />
  );
}

function TranslatedMessage({
  message,
  t,
}: {
  message?: string;
  t?: ReturnType<typeof useTranslation>["t"];
}) {
  const fallback = useTranslation().t;
  if (!message) return null;
  return <p className="text-sm text-destructive">{(t ?? fallback)(message)}</p>;
}

function headerInputID(header: string) {
  return `header-${header.replace(/[^A-Za-z0-9_-]/g, "-")}`;
}
