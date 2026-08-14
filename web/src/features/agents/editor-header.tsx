import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { FlaskConical, History } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";

/**
 * Which agent is being written, and the two places to go from here.
 *
 * Fixed above the tabs, so the answer to "what am I editing" never scrolls
 * away — and neither does the fact that publishing writes a new version
 * rather than changing the one runs are pinned to.
 */
export function EditorHeader({
  agentId,
  name,
  creating,
  version,
}: {
  agentId: string;
  name: string;
  creating: boolean;
  version?: string;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex h-13 shrink-0 items-center gap-3 border-b border-border px-4">
      <div className="flex min-w-0 items-center gap-2">
        <h1 className="truncate text-sm font-medium">
          {name || t(creating ? "agents.newAgent" : "agents.untitled")}
        </h1>
        {agentId !== "" && <Mono dim className="truncate text-2xs">{agentId}</Mono>}
        {version && (
          <Badge variant="outline" className="shrink-0">
            <Mono className="text-2xs">{version.slice(0, 9)}</Mono>
          </Badge>
        )}
      </div>

      {!creating && agentId !== "" && (
        <div className="ml-auto flex shrink-0 items-center gap-1.5">
          <Button variant="ghost" size="sm" asChild className="h-8">
            <Link to={`/agents/${agentId}`}>
              <History className="size-4" aria-hidden />
              {t("agents.versionsItem")}
            </Link>
          </Button>
          <Button variant="outline" size="sm" asChild className="h-8">
            <Link to={`/agents/${agentId}/simulate`}>
              <FlaskConical className="size-4" aria-hidden />
              {t("agents.simulate")}
            </Link>
          </Button>
        </div>
      )}
    </div>
  );
}
