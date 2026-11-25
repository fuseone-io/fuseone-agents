import { useTranslation } from "react-i18next";
import { LogIn } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { problemMessage } from "@/lib/api/problem-message";
import { useStartMCPSignIn } from "@/features/integrations/api";
import { sendBrowserTo } from "@/features/integrations/mcp/browser-navigation";

/**
 * Connects the signed-in person's own account to a server people sign in to:
 * the sign-in begins here and the browser goes to the provider. Nothing is
 * stored until it comes back to Credentials.
 */
export function BrowserSignIn({ server }: { server: string }) {
  const { t } = useTranslation();
  const start = useStartMCPSignIn();

  async function connect() {
    try {
      const { authorizeURL } = await start.mutateAsync(server);
      sendBrowserTo(authorizeURL);
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  return (
    <div className="space-y-2 rounded-lg border bg-muted p-3">
      <p className="text-xs text-muted-foreground">{t("mcp.signIn.connectHint")}</p>
      <Button type="button" onClick={() => void connect()} disabled={start.isPending}>
        <LogIn className="size-4" />
        {start.isPending ? t("mcp.signIn.connecting") : t("mcp.signIn.connect")}
      </Button>
    </div>
  );
}
