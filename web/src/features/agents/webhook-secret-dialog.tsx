import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Check, Copy, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/**
 * The secret, shown once.
 *
 * What the platform keeps is a hash, so there is no screen that can show this
 * again — the dialog says so plainly rather than letting somebody close it and
 * find out later. Copy is the only action, because reading a 43-character
 * secret off a screen is how the wrong one ends up in a configuration file.
 */
export function WebhookSecretDialog({
  secret,
  url,
  onClose,
}: {
  secret?: string;
  url?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(secret ?? "");
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <Dialog open={Boolean(secret)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("agents.webhookKey")}</DialogTitle>
          <DialogDescription>{t("agents.configureNow")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          <Field label="Endereço">
            <code className="block truncate rounded-lg border border-border bg-muted px-3 py-2 font-mono text-xs">
              POST {url}
            </code>
          </Field>

          <Field label="Cabeçalho X-FuseOne-Secret">
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-lg border border-border bg-muted px-3 py-2 font-mono text-xs">
                {secret}
              </code>
              <Button variant="outline" size="sm" onClick={() => void copy()}>
                {copied ? (
                  <Check className="size-4" />
                ) : (
                  <Copy className="size-4" />
                )}
                {copied ? "Copiado" : "Copiar"}
              </Button>
            </div>
          </Field>

          <p className="flex items-start gap-2 text-xs text-muted-foreground">
            <TriangleAlert
              className="mt-px size-3.5 shrink-0 text-warning"
              aria-hidden
            />
            <span>
              {t("agents.callerAlsoNeeds")}
              <code className="font-mono">
                {/* eslint-disable-next-line i18next/no-literal-string */}
                Idempotency-Key
              </code>{" "}
              único por entrega, repetindo o mesmo valor em cada nova tentativa
              daquela entrega. Sem isso, uma reentrega abre uma segunda
              execução.
            </span>
          </p>
        </div>

        <DialogFooter>
          <Button onClick={onClose}>{t("agents.done")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </span>
      {children}
    </div>
  );
}
