import { useTranslation } from "react-i18next";
import { LogIn } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AuthLayout } from "@/features/session/auth-layout";
import type { IdentityProvider } from "@/features/session/providers";

/**
 * Sign-in is a redirect, not a form: the console never sees a password. Each
 * button hands the browser to the provider, which hands it back to a callback
 * that issues the session.
 */
export function SignInPage({ providers }: { providers: IdentityProvider[] }) {
  const { t } = useTranslation();
  const returnTo = globalThis.location.pathname + globalThis.location.search;

  return (
    <AuthLayout
      icon={<LogIn className="size-5 text-primary" />}
      title="Entrar"
      description="Use a conta da sua organização. O acesso que você recebe vem dos grupos que ela informa."
    >
      {providers.length === 0 ? (
        <Alert>
          <AlertDescription>{t("session.noProvider")}</AlertDescription>
        </Alert>
      ) : (
        <div className="flex flex-col gap-2">
          {providers.map((provider) => (
            <Button
              key={provider.id}
              asChild
              variant="outline"
              className="justify-start"
            >
              <a
                href={`/auth/start/${provider.id}?returnTo=${encodeURIComponent(returnTo)}`}
              >
                {t("session.signInWith", {
                  provider: provider.display || provider.id,
                })}
              </a>
            </Button>
          ))}
        </div>
      )}
    </AuthLayout>
  );
}
