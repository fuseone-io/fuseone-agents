import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { problemMessage } from "@/lib/api/problem-message";
import { RediscoveryStatus } from "@/features/integrations/mcp/rediscovery-status";
import { useRequestMCPRediscovery } from "@/features/integrations/mcp/rediscovery-api";

/**
 * Asking every worker to look at a server again (#287), and who has.
 *
 * Beside Try now and not instead of it: Try now is one worker reconnecting to
 * report what it reaches; this is every worker asking again, on the session it
 * holds, with nothing reconnected. The confirmation says so, because the two
 * read alike and do different things.
 */
export function RediscoveryPanel({ server, enabled }: { server: string; enabled: boolean }) {
  const { t } = useTranslation();
  const request = useRequestMCPRediscovery(server);

  async function ask() {
    try {
      await request.mutateAsync();
      toast.success(t("mcp.rediscovery.requested"));
    } catch (problem) {
      toast.error(problemMessage(problem, t));
    }
  }

  return (
    <Panel
      title={t("mcp.rediscovery.title")}
      action={
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="outline" size="sm" disabled={!enabled || request.isPending}>
              {t("mcp.rediscovery.ask")}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("mcp.rediscovery.confirmTitle")}</AlertDialogTitle>
              <AlertDialogDescription>{t("mcp.rediscovery.confirmBody")}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
              <AlertDialogAction onClick={() => void ask()}>
                {t("mcp.rediscovery.confirm")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      }
    >
      <div className="space-y-3 p-4">
        <p className="text-xs text-muted-foreground">{t("mcp.rediscovery.is")}</p>
        <RediscoveryStatus server={server} />
      </div>
    </Panel>
  );
}
