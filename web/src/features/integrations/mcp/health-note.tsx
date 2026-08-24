import { Activity } from "lucide-react";
import { Trans, useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { formatRelative } from "@/lib/format";
import type { MCPServer } from "@/features/integrations/api";

export function MCPHealthNote({ health }: { health?: MCPServer["health"] | null }) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2 rounded-lg border bg-muted p-3">
      <Activity className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="space-y-2">
        <p className="text-xs font-medium">{t("mcp.runtimeHealthTitle")}</p>
        <p className="text-xs text-muted-foreground">
          {discoveryMessage(health, t)}
        </p>
        <p className="text-xs text-muted-foreground">
          {toolCallMessage(health, t)}
        </p>
      </div>
    </div>
  );
}

function discoveryMessage(
  health: MCPServer["health"] | null | undefined,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!health) return t("mcp.discoveryNeverObserved");
  if (!health.reachable) {
    const last = health.lastReachableAt
      ? formatRelative(health.lastReachableAt)
      : t("mcp.never");
    return t("mcp.discoveryFailed", {
      seen: formatRelative(health.observedAt),
      last,
    });
  }
  return t("mcp.discoveryAnswered", {
    count: health.toolCount,
    seen: formatRelative(health.observedAt),
  });
}

function toolCallMessage(
  health: MCPServer["health"] | null | undefined,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!health?.toolCall) return t("mcp.toolCallNeverObserved");
  if (health.toolCall.ok) {
    return t("mcp.toolCallAnswered", {
      seen: formatRelative(health.toolCall.observedAt),
    });
  }
  const last = health.toolCall.lastOkAt
    ? formatRelative(health.toolCall.lastOkAt)
    : t("mcp.never");
  return (
    <Trans
      i18nKey="mcp.toolCallFailed"
      values={{
        code: health.toolCall.code,
        seen: formatRelative(health.toolCall.observedAt),
        last,
      }}
      components={{ code: <Mono dim /> }}
    />
  );
}
