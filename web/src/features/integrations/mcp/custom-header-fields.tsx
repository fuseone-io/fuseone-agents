import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { CustomHeader } from "@/features/integrations/mcp/custom-header";

/**
 * A header name and its value, for a remote server whose key is not a bearer
 * token. The value is a secret and never comes back: with a credential stored,
 * the fields start empty and filling them replaces it.
 */
export function CustomHeaderFields({
  value,
  onChange,
  kept = false,
  error,
}: {
  value: CustomHeader;
  onChange: (next: CustomHeader) => void;
  kept?: boolean;
  error?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p className="text-sm font-medium">{t("mcp.customHeader")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="min-w-0 space-y-1.5">
          <Label htmlFor="custom-header-name">{t("mcp.customHeaderName")}</Label>
          <Input
            id="custom-header-name"
            className="font-mono"
            autoComplete="off"
            placeholder={t("mcp.customHeaderNamePlaceholder")}
            value={value.name}
            onChange={(e) => onChange({ ...value, name: e.target.value })}
          />
        </div>
        <div className="min-w-0 space-y-1.5">
          <Label htmlFor="custom-header-value">{t("mcp.customHeaderValue")}</Label>
          <Input
            id="custom-header-value"
            type="password"
            className="font-mono"
            autoComplete="off"
            value={value.value}
            onChange={(e) => onChange({ ...value, value: e.target.value })}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {kept ? t("integrations.tokenKept") : t("mcp.customHeaderHint")}
      </p>
      {error && <p className="text-xs text-danger">{t(error)}</p>}
    </div>
  );
}
