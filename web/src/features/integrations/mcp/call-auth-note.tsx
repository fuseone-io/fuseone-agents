import { KeyRound } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { MCPServer } from "@/features/integrations/api";

export function CallAuthNote({
  callAuth,
}: {
  callAuth?: MCPServer["callAuth"] | null;
}) {
  const { t } = useTranslation();
  if (!callAuth) return null;

  const tone = toneOf(callAuth);
  return (
    <div className={cn("flex gap-2 rounded-lg border p-3", tone.shell)}>
      <KeyRound className={cn("mt-0.5 size-4 shrink-0", tone.icon)} />
      <div className="space-y-1">
        <p className="text-xs font-medium">{t("mcp.callAuthTitle")}</p>
        <p className="text-xs text-muted-foreground">{messageOf(callAuth, t)}</p>
      </div>
    </div>
  );
}

function messageOf(
  callAuth: NonNullable<MCPServer["callAuth"]>,
  t: ReturnType<typeof useTranslation>["t"],
) {
  switch (callAuth.policy) {
    case "personal_required":
      return callAuth.callerHasPersonalCredential
        ? t("mcp.callAuthPersonalReady")
        : t("mcp.callAuthPersonalMissing");
    case "installation_or_service":
      return callAuth.callerHasPersonalCredential
        ? t("mcp.callAuthSharedWithPersonal")
        : t("mcp.callAuthShared");
    case "local_process":
      return t("mcp.callAuthLocal");
    default:
      return callAuth.callerHasPersonalCredential
        ? t("mcp.callAuthUnknownWithPersonal")
        : t("mcp.callAuthUnknown");
  }
}

function toneOf(callAuth: NonNullable<MCPServer["callAuth"]>) {
  if (callAuth.policy === "personal_required" && !callAuth.callerHasPersonalCredential) {
    return { shell: "border-warning/40 bg-warning-surface", icon: "text-warning" };
  }
  if (callAuth.policy === "unknown") {
    return { shell: "border-warning/30 bg-warning-surface/60", icon: "text-warning" };
  }
  return { shell: "bg-muted", icon: "text-muted-foreground" };
}
