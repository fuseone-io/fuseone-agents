import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import type { ServiceToken } from "@/features/admin/service-accounts-api";

/**
 * The secret, shown once.
 *
 * Only its hash is kept, so nothing can show this again. That is why it has no
 * dismissal but the button: Escape or a stray click would lose a credential
 * for good, and the only way back is another token. Shown whole rather than
 * truncated, because a console on plain HTTP has no clipboard to copy with.
 */
export function ServiceTokenSecret({
  secret,
  replacing,
  onClose,
}: {
  secret: string;
  replacing?: ServiceToken;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast.error(t("serviceAccounts.copyFailed"));
    }
  };

  return (
    <AlertDialog open>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("serviceAccounts.secretTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("serviceAccounts.secretExplains")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="flex items-start gap-2">
          <code className="min-w-0 flex-1 break-all rounded-lg border border-border bg-muted px-3 py-2 font-mono text-xs">
            {secret}
          </code>
          <Button variant="outline" size="sm" onClick={() => void copy()}>
            {copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}
            {t(copied ? "serviceAccounts.copied" : "serviceAccounts.copy")}
          </Button>
        </div>
        {replacing && (
          <p className="text-xs text-muted-foreground">
            {t("serviceAccounts.secretRotated", { id: replacing.id })}
          </p>
        )}
        <AlertDialogFooter>
          <Button onClick={onClose}>{t("serviceAccounts.secretSaved")}</Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
