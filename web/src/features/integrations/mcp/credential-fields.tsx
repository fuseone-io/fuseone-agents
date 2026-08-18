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
  hasConfigFile,
  showRemoteToken = true,
  remoteTokenLabel,
  remoteTokenHint,
  remoteHeaders = [],
  remoteHeadersHint,
  dsnLabel,
  dsnHint,
  value,
  onChange,
  onRevoke,
  onRevokeConfigFile,
}: {
  local: boolean;
  hasSecret: boolean;
  hasConfigFile: boolean;
  showRemoteToken?: boolean;
  remoteTokenLabel?: string;
  remoteTokenHint?: string;
  remoteHeaders?: string[];
  remoteHeadersHint?: string;
  dsnLabel?: string;
  dsnHint?: string;
  value: {
    token: string;
    headers: Record<string, string>;
    dsn: string;
    env: string;
    configFile: string;
    configFileEnv: string;
  };
  onChange: (next: {
    token: string;
    headers: Record<string, string>;
    dsn: string;
    env: string;
    configFile: string;
    configFileEnv: string;
  }) => void;
  onRevoke: () => void;
  onRevokeConfigFile: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      {local ? (
        <div className="space-y-4">
          {dsnLabel !== undefined && (
            <div className="space-y-1.5">
              <Label htmlFor="dsn">{dsnLabel}</Label>
              <Input
                id="dsn"
                type="password"
                autoComplete="off"
                className="font-mono"
                value={value.dsn}
                onChange={(e) => onChange({ ...value, dsn: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                {hasSecret ? t("mcp.variablesKept") : dsnHint}
              </p>
            </div>
          )}
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
              {hasSecret ? t("mcp.variablesKept") : t("mcp.variablesHint")}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="config-file-env">{t("mcp.configFileEnv")}</Label>
            <Input
              id="config-file-env"
              className="font-mono"
              placeholder="FUSEONE_MCP_CONFIG_FILE"
              value={value.configFileEnv}
              onChange={(e) =>
                onChange({ ...value, configFileEnv: e.target.value })
              }
            />
            <p className="text-xs text-muted-foreground">
              {t("mcp.configFileEnvHint")}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="config-file">{t("mcp.configFile")}</Label>
            <Textarea
              id="config-file"
              rows={6}
              className="font-mono text-xs"
              placeholder={t("mcp.configFileExample")}
              value={value.configFile}
              onChange={(e) =>
                onChange({ ...value, configFile: e.target.value })
              }
            />
            <p className="text-xs text-muted-foreground">
              {hasConfigFile ? t("mcp.configFileKept") : t("mcp.configFileHint")}
            </p>
          </div>
        </div>
      ) : showRemoteToken || remoteHeaders.length > 0 ? (
        <div className="space-y-4">
          {showRemoteToken && (
            <div className="space-y-1.5">
              <Label htmlFor="token">{remoteTokenLabel ?? t("integrations.token")}</Label>
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
                  : remoteTokenHint ?? t("integrations.tokenHint")}
              </p>
            </div>
          )}
          {remoteHeaders.length > 0 && (
            <div className="space-y-3">
              <div className="grid gap-3 sm:grid-cols-2">
                {remoteHeaders.map((header) => (
                  <div key={header} className="space-y-1.5">
                    <Label htmlFor={headerInputID(header)}>{header}</Label>
                    <Input
                      id={headerInputID(header)}
                      type="password"
                      autoComplete="off"
                      className="font-mono"
                      value={value.headers[header] ?? ""}
                      onChange={(e) =>
                        onChange({
                          ...value,
                          headers: { ...value.headers, [header]: e.target.value },
                        })
                      }
                    />
                  </div>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                {hasSecret
                  ? t("integrations.tokenKept")
                  : remoteHeadersHint ?? t("mcp.remoteHeadersHint")}
              </p>
            </div>
          )}
        </div>
      ) : hasSecret ? (
        <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
          {t("mcp.storedBearerOutsideRecipe")}
        </p>
      ) : (
        <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
          {t("mcp.noBearerFieldForRecipe")}
        </p>
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
          {local ? t("mcp.revokeVariables") : t("mcp.revoke")}
        </Button>
      )}
      {local && hasConfigFile && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRevokeConfigFile}
          className="text-danger"
        >
          <Trash2 className="size-3.5" />
          {t("mcp.revokeConfigFile")}
        </Button>
      )}
    </div>
  );
}

function headerInputID(header: string) {
  return `header-${header.replace(/[^A-Za-z0-9_-]/g, "-")}`;
}
