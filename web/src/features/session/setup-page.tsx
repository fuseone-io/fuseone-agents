import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { AuthLayout } from "@/features/session/auth-layout";

const schema = z.object({
  token: z.string().min(1, "session.pasteToken"),
  display: z.string().min(1, "session.sayWhoYouAre"),
});

/**
 * The first run.
 *
 * A new installation is a deadlock: configuring an identity provider needs the
 * Curator permission, and the only way to get a Curator is through an identity
 * provider. The setup token breaks it exactly once, and this screen is where.
 */
export function SetupPage({ onClaimed }: { onClaimed: () => void }) {
  const { t } = useTranslation();
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { token: "", display: "" },
  });

  async function submit(values: z.infer<typeof schema>) {
    const response = await fetch("/auth/bootstrap", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    });

    if (!response.ok) {
      const problem = await response.json().catch(() => undefined);
      form.setError("token", {
        message: problem?.detail ?? t("session.setupFailed"),
      });
      return;
    }
    onClaimed();
  }

  return (
    <AuthLayout
      icon={<ShieldCheck className="size-5 text-primary" />}
      title={t("session.setupTitle")}
      description={t("session.setupOnce")}
    >
      <Alert>
        <AlertDescription>{t("session.tokenInLog")}</AlertDescription>
      </Alert>

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex flex-col gap-4"
        >
          <FormField
            control={form.control}
            name="token"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("session.setupToken")}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="display"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t("session.yourName")}</FormLabel>
                <FormControl>
                  <Input {...field} autoComplete="name" />
                </FormControl>
                <FormDescription>
                  {t("session.recordedAsClaimant")}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type="submit" disabled={form.formState.isSubmitting}>
            {t("session.finishSetup")}
          </Button>
        </form>
      </Form>
    </AuthLayout>
  );
}
