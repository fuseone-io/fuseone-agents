import type { TFunction } from "i18next";
import type { AuthMode } from "@/features/integrations/mcp/auth-plan";

export function remoteTokenLabel(mode: AuthMode | null, t: TFunction) {
  return mode?.label ?? t("mcp.remoteBearerToken");
}

export function remoteTokenHint(mode: AuthMode | null, t: TFunction) {
  if (!mode) return undefined;
  const header = mode.header ?? "Authorization";
  const prefix = mode.prefix;
  if (!prefix) return t("mcp.remoteHeaderHint", { header });
  return t("mcp.remoteBearerHint", { header, prefix });
}
