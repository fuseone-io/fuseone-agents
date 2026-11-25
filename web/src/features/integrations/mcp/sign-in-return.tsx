import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";

const OUTCOMES = ["connected", "denied", "expired", "not_yours", "changed", "refused", "failed"] as const;

/**
 * What the browser brings back to Credentials from a server's sign-in: the
 * outcome, said once, and the server it was for, chosen. Taken out of the
 * address afterwards, so a reload says nothing again. Only codes from the
 * closed set the server answers are said; anything else is ignored.
 */
export function SignInReturn({ onServer }: { onServer: (server: string) => void }) {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const said = useRef(false);

  useEffect(() => {
    const server = params.get("server");
    if (server) onServer(server);
    const outcome = params.get("oauth");
    if (said.current || !outcome) return;
    said.current = true;
    if ((OUTCOMES as readonly string[]).includes(outcome)) {
      const message = t(`mcp.signIn.return.${outcome}`, { server: server ?? "" });
      if (outcome === "connected") toast.success(message);
      else toast.error(message);
    }
    const next = new URLSearchParams(params);
    next.delete("oauth");
    next.delete("server");
    setParams(next, { replace: true });
  }, [params, setParams, onServer, t]);

  return null;
}
