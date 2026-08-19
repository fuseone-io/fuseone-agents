import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { CredentialValue } from "@/features/integrations/mcp/credential-value";

export function OAuthFields({
  value,
  hasOAuth,
  conflict,
  invalidExpiry,
  onChange,
  onRevoke,
}: {
  value: CredentialValue;
  hasOAuth: boolean;
  conflict: boolean;
  invalidExpiry: boolean;
  onChange: (next: CredentialValue) => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 border-t pt-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <OAuthInput
          id="oauth-access-token"
          label={t("mcp.oauthAccessToken")}
          type="password"
          value={value.oauthAccessToken}
          onChange={(oauthAccessToken) => onChange({ ...value, oauthAccessToken })}
        />
        <OAuthInput
          id="oauth-refresh-token"
          label={t("mcp.oauthRefreshToken")}
          type="password"
          value={value.oauthRefreshToken}
          onChange={(oauthRefreshToken) => onChange({ ...value, oauthRefreshToken })}
        />
        <OAuthInput
          id="oauth-token-url"
          label={t("mcp.oauthTokenURL")}
          value={value.oauthTokenURL}
          onChange={(oauthTokenURL) => onChange({ ...value, oauthTokenURL })}
        />
        <OAuthInput
          id="oauth-client-id"
          label={t("mcp.oauthClientID")}
          value={value.oauthClientID}
          onChange={(oauthClientID) => onChange({ ...value, oauthClientID })}
        />
        <OAuthInput
          id="oauth-client-secret"
          label={t("mcp.oauthClientSecret")}
          type="password"
          value={value.oauthClientSecret}
          onChange={(oauthClientSecret) => onChange({ ...value, oauthClientSecret })}
        />
        <OAuthInput
          id="oauth-token-type"
          label={t("mcp.oauthTokenType")}
          placeholder={t("mcp.oauthTokenTypePlaceholder")}
          value={value.oauthTokenType}
          onChange={(oauthTokenType) => onChange({ ...value, oauthTokenType })}
        />
        <OAuthInput
          id="oauth-expires-at"
          label={t("mcp.oauthExpiresAtUnix")}
          value={value.oauthExpiresAtUnix}
          onChange={(oauthExpiresAtUnix) => onChange({ ...value, oauthExpiresAtUnix })}
        />
        <div className="space-y-1.5">
          <Label htmlFor="oauth-scopes">{t("mcp.oauthScopes")}</Label>
          <Textarea
            id="oauth-scopes"
            rows={2}
            className="font-mono text-xs"
            value={value.oauthScopes}
            onChange={(e) => onChange({ ...value, oauthScopes: e.target.value })}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {hasOAuth ? t("mcp.oauthKept") : t("mcp.oauthHint")}
      </p>
      {conflict && (
        <p className="text-xs text-danger">{t("mcp.oauthBearerConflict")}</p>
      )}
      {invalidExpiry && (
        <p className="text-xs text-danger">{t("mcp.oauthExpiryInvalid")}</p>
      )}
      {hasOAuth && (
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

function OAuthInput({
  id,
  label,
  type = "text",
  placeholder,
  value,
  onChange,
}: {
  id: string;
  label: string;
  type?: string;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        autoComplete="off"
        className="font-mono"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}
