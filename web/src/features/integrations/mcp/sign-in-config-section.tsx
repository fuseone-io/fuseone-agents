import { useId } from "react";
import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Copy, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Separator } from "@/components/ui/separator";
import { Mono } from "@/components/shared/mono";
import type { MCPServer } from "@/features/integrations/api";
import { SignInConfigFields } from "@/features/integrations/mcp/sign-in-config-fields";
import {
  signInCallbackURL,
  signInConfigSchema,
  signInDefaults,
  signInInput,
  withoutSecret,
  type SignInConfigValues,
} from "@/features/integrations/mcp/sign-in-config";
import type { ServerConnection } from "@/features/integrations/mcp/use-server-credential-write";

/**
 * How people sign in to this server from their browser, configured once by
 * whoever configures the server. Each person then connects their own account
 * in Credentials, and their agents call the server as them. The secret is
 * never shown back: the page says only that one is held.
 */
export function SignInConfigSection({ server, connection }: { server: MCPServer; connection: ServerConnection }) {
  const { t } = useTranslation();
  const titleID = useId();
  const stored = server.oauthLogin;
  const callback = signInCallbackURL(window.location.origin);
  const form = useForm<SignInConfigValues>({
    resolver: zodResolver(signInConfigSchema),
    defaultValues: signInDefaults(stored),
  });

  async function save(values: SignInConfigValues) {
    if (await connection.write({ oauthLogin: signInInput(values) })) form.setValue("clientSecret", "");
  }

  // The method goes back to the default with the secret, in the form too: a
  // save after it must not send a method that needs a secret with none held.
  async function removeSecret(login: NonNullable<typeof stored>) {
    if (!(await connection.write({ oauthLogin: withoutSecret(login) }))) return;
    form.setValue("clientSecret", "");
    form.setValue("tokenAuthMethod", "auto");
  }

    async function copy() {
    try {
      await navigator.clipboard.writeText(callback);
      toast.success(t("mcp.signIn.copied"));
    } catch {
      toast.error(t("mcp.signIn.copyFailed"));
    }
  }

  return (
    <section aria-labelledby={titleID} className="space-y-3">
      <Separator />
      <h3 id={titleID} className="text-sm font-medium">{t("mcp.signIn.title")}</h3>
      <p className="text-xs text-muted-foreground">{t("mcp.signIn.hint")}</p>
      <div className="flex min-w-0 items-center gap-2 rounded-lg border bg-muted px-3 py-2">
        <div className="min-w-0 flex-1">
          <p className="text-xs text-muted-foreground">{t("mcp.signIn.callback")}</p>
          <Mono className="block truncate text-xs">{callback}</Mono>
        </div>
        <Button type="button" variant="ghost" size="icon" aria-label={t("mcp.signIn.copyCallback")} onClick={() => void copy()}>
          <Copy className="size-3.5" />
        </Button>
      </div>
      {stored && (
        <p className="text-xs text-muted-foreground">
          {t(stored.hasClientSecret ? "mcp.signIn.storedWithSecret" : "mcp.signIn.stored", { clientID: stored.clientID })}
        </p>
      )}
      <Form {...form}>
        <form onSubmit={form.handleSubmit(save)} className="space-y-3">
          <SignInConfigFields form={form} />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="outline" size="sm" disabled={connection.pending}>
              {t("mcp.signIn.save")}
            </Button>
            {stored?.hasClientSecret && (
              <Button type="button" variant="outline" size="sm" className="text-danger" disabled={connection.pending}
                onClick={() => void removeSecret(stored)}>
                <Trash2 className="size-3.5" />
                {t("mcp.signIn.removeSecret")}
              </Button>
            )}
            {stored && (
              <Button type="button" variant="outline" size="sm" className="text-danger" disabled={connection.pending}
                onClick={() => void connection.write({ oauthLogin: {} })}>
                <Trash2 className="size-3.5" />
                {t("mcp.signIn.remove")}
              </Button>
            )}
          </div>
        </form>
      </Form>
    </section>
  );
}
