import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

/**
 * What a server is given, and how to take it back.
 *
 * A bearer for an address, variables for a program — never both, because the
 * platform drops the half the transport cannot use and a form offering both
 * would be promising to keep something it will discard.
 *
 * Removing is its own gesture. An empty field means "leave what is stored", or
 * correcting an address would demand re-entering a secret nobody has to hand;
 * with only that rule a credential could be written and never revoked, which
 * is the half that matters when it has leaked.
 */
export function CredentialFields({
  local,
  hasSecret,
  value,
  onChange,
  onRevoke,
}: {
  local: boolean;
  hasSecret: boolean;
  value: { token: string; env: string };
  onChange: (next: { token: string; env: string }) => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      {local ? (
        <div className="space-y-1.5">
          <Label htmlFor="env">{t("mcp.variables")}</Label>
          <Textarea
            id="env"
            rows={4}
            className="font-mono text-xs"
            placeholder={t("mcp.variablesExample")}
            value={value.env}
            onChange={(e) => onChange({ ...value, env: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">
            {t("mcp.variablesHint")}
          </p>
        </div>
      ) : (
        <div className="space-y-1.5">
          <Label htmlFor="token">{t("integrations.token")}</Label>
          <Input
            id="token"
            type="password"
            autoComplete="off"
            className="font-mono"
            value={value.token}
            onChange={(e) => onChange({ ...value, token: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">
            {hasSecret
              ? t("integrations.tokenKept")
              : t("integrations.tokenHint")}
          </p>
        </div>
      )}

      {hasSecret && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRevoke}
          className="text-danger"
        >
          <Trash2 className="size-3.5" />
          {t("mcp.revoke")}
        </Button>
      )}
    </div>
  );
}
