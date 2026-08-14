import { useTranslation } from "react-i18next";
import { LogIn } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AuthLayout } from "@/features/session/auth-layout";
import { LocalSignIn } from "@/features/session/local-sign-in";
import { Separator } from "@/components/ui/separator";
import type { IdentityProvider } from "@/features/session/providers";

/**
 * Sign-in is a redirect, not a form: the console never sees a password. Each
 * button hands the browser to the provider, which hands it back to a callback
 * that issues the session.
 */
export function SignInPage({
  providers,
  localSignIn,
  onSignedIn,
}: {
  providers: IdentityProvider[];
  localSignIn: boolean;
  onSignedIn: () => void;
}) {
  const { t } = useTranslation();
  const returnTo = globalThis.location.pathname + globalThis.location.search;

  return (
    <AuthLayout
      icon={<LogIn className="size-5 text-primary" />}
      title={t("session.signIn")}
      description={t("session.useOrgAccount")}
    >
      {providers.length === 0 && !localSignIn ? (
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

      {/* Below the providers, never above: where a provider exists it is how
          people should arrive, and this is the door that stays open when it
          does not. */}
      {localSignIn && (
        <div className="mt-4 flex flex-col gap-4">
          {providers.length > 0 && (
            <div className="flex items-center gap-3">
              <Separator className="flex-1" />
              <span className="text-2xs uppercase tracking-label text-muted-foreground">
                {t("session.orProvider")}
              </span>
              <Separator className="flex-1" />
            </div>
          )}
          <LocalSignIn onSignedIn={onSignedIn} />
        </div>
      )}
    </AuthLayout>
  );
}
