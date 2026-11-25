import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { CredentialValue } from "@/features/integrations/mcp/credential-value";
import { OAuthGrantFields } from "@/features/integrations/mcp/oauth-grant-fields";
import { oauthHasValue, oauthIsUsable } from "@/features/integrations/mcp/oauth-credential";
import type { MCPOAuthClient } from "@/features/integrations/api";

export function OAuthFields({
  value,
  stored,
  invalidExpiry,
  onChange,
  onRevoke,
}: {
  value: CredentialValue;
  /** Whether a grant is stored, and whose client it is when the read says. */
  stored: { hasOAuth: boolean; client?: MCPOAuthClient };
  invalidExpiry: boolean;
  onChange: (next: CredentialValue) => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  const client = stored.hasOAuth ? stored.client : undefined;
  return (
    <div className="space-y-3 border-t pt-3">
      <p className="text-xs text-muted-foreground">
        {stored.hasOAuth ? t("mcp.oauthKept") : t("mcp.oauthHint")}
      </p>
      {client?.tokenURL && (
        <p className="text-xs text-muted-foreground">
          <Mono className="text-xs">
            {t("mcp.oauthStoredClient", { tokenURL: client.tokenURL, clientID: client.clientID ?? "—" })}
          </Mono>
        </p>
      )}
      <OAuthGrantFields value={value} onChange={(field, next) => onChange({ ...value, [field]: next })} />
      {invalidExpiry && (
        <p className="text-xs text-danger">{t("mcp.oauthExpiryInvalid")}</p>
      )}
      {oauthHasValue(value) && !oauthIsUsable(value) && (
        <p className="text-xs text-danger">{t("mcp.oauthNothingToSend")}</p>
      )}
      {stored.hasOAuth && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRevoke}
          className="text-danger"
        >
          <Trash2 className="size-3.5" />
          {t("mcp.revokeOAuth")}
        </Button>
      )}
    </div>
  );
}

export function StoredOAuthOnly({ onRevoke }: { onRevoke: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 border-t pt-3">
      <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
        {t("mcp.storedOAuthOutsideRecipe")}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRevoke}
        className="text-danger"
      >
        <Trash2 className="size-3.5" />
        {t("mcp.revokeOAuth")}
      </Button>
    </div>
  );
}
