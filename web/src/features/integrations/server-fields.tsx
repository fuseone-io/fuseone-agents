import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import { dsnEnvMode } from "@/features/integrations/mcp/auth-plan";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import type { MCPServer } from "@/features/integrations/api";
import { ServerAuthFields } from "@/features/integrations/mcp/server-auth-fields";

/**
 * The half of the form that depends on how the server is reached.
 *
 * A local server is a command this installation runs; a remote one is an
 * address it calls. Showing both at once would invite filling both, and only
 * one of them is ever used.
 */
export function ServerFields({
  form,
  server,
  recipe,
}: {
  form: UseFormReturn<ServerFormValues>;
  /** The server as stored, or null for one being connected. */
  server?: MCPServer | null;
  recipe?: ServerRecipe | null;
}) {
  const { t } = useTranslation();
  const hasSecret = server?.hasSecret === true;
  const hasConfigFile = server?.hasConfigFile === true;

  if (form.watch("transport") === "http") {
    return (
      <>
        <Field
          form={form}
          name="url"
          label={t("integrations.url")}
          placeholder="https://api.example.com/mcp/"
        />
        <ServerAuthFields form={form} server={server} recipe={recipe} />
        <RateLimitFields form={form} />
        <ResultCacheFields form={form} />
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
      <StdioEgressFields form={form} />
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
      <ResultCacheFields form={form} />
      <AcceptLocalExecution form={form} />
    </>
  );
}

function StdioEgressFields({
  form,
}: {
  form: UseFormReturn<ServerFormValues>;
}) {
  const { t } = useTranslation();
  const mode = form.watch("stdioEgressMode");
  return (
    <div className="space-y-3 rounded-lg border border-warning/30 bg-warning-surface/50 p-3">
      <FormField
        control={form.control}
        name="stdioEgressMode"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("mcp.stdioEgressMode")}</FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="inherit">
                  {t("mcp.stdioEgressInherit")}
                </SelectItem>
                <SelectItem value="proxied">
                  {t("mcp.stdioEgressProxied")}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormDescription>{t("mcp.stdioEgressHint")}</FormDescription>
          </FormItem>
        )}
      />
      {mode === "proxied" && (
        <TextAreaField
          form={form}
          name="stdioEgressDestinations"
          label={t("mcp.stdioEgressDestinations")}
          placeholder={t("mcp.stdioEgressDestinationsExample")}
          hint={t("mcp.stdioEgressDestinationsHint")}
        />
      )}
    </div>
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
        <Field
          form={form}
          name="callTimeoutSeconds"
          label={t("mcp.callTimeout")}
          placeholder="60"
        />
      </div>
      <p className="text-xs text-muted-foreground">{t("mcp.rateLimitHint")}</p>
    </div>
  );
}

function ResultCacheFields({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          form={form}
          name="cacheTTLSeconds"
          label={t("mcp.cacheTTLSeconds")}
          placeholder="30"
        />
        <Field
          form={form}
          name="cacheMaxEntries"
          label={t("mcp.cacheMaxEntries")}
          placeholder="256"
        />
      </div>
      <p className="text-xs text-muted-foreground">{t("mcp.cacheHint")}</p>
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
    | "callTimeoutSeconds"
    | "rateLimitBurst"
    | "cacheTTLSeconds"
    | "cacheMaxEntries"
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
  name: "configFile" | "oauthScopes" | "env" | "stdioEgressDestinations";
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
