import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

/**
 * Signing in with a username and password.
 *
 * The path that exists so an installation cannot lock itself out: a fresh one
 * has no identity provider, and until somebody registers one this is the only
 * way in. It sits below the providers, not above them, because where a
 * provider exists it is the way people should arrive.
 *
 * One message for every refusal. Saying which half was wrong turns the form
 * into a way to find out who exists here.
 */
export function LocalSignIn({ onSignedIn }: { onSignedIn: () => void }) {
  const { t } = useTranslation();
  const [failed, setFailed] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setFailed(null);
    try {
      const response = await fetch("/auth/local", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          username: String(form.get("username") ?? ""),
          password: String(form.get("password") ?? ""),
        }),
      });
      if (!response.ok) {
        setFailed(
          response.status === 429
            ? t("session.tooManyAttempts")
            : t("session.signInFailed"),
        );
        return;
      }
      onSignedIn();
    } catch {
      setFailed(t("common.requestFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      {failed && (
        <Alert variant="destructive">
          <AlertDescription>{failed}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="username">{t("session.usernameLabel")}</Label>
        <Input id="username" name="username" autoComplete="username" required />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="password">{t("session.passwordLabel")}</Label>
        <Input
          id="password"
          name="password"
          type="password"
          autoComplete="current-password"
          required
        />
      </div>
      <Button type="submit" disabled={busy}>
        {t("session.signIn")}
      </Button>
    </form>
  );
}
