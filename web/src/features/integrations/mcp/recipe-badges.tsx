import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

type RecipeStatus = ServerRecipe["status"];
type ConfigRequirement = ServerRecipe["configRequirements"][number];
type AuthMode = NonNullable<ServerRecipe["authModes"]>[number];

const STATUS: Record<RecipeStatus, string> = {
  published: "bg-muted text-muted-foreground",
  reference: "bg-warning-surface text-warning",
  archived: "bg-danger-surface text-danger",
};

export function RecipeStatusBadge({ status }: { status: RecipeStatus }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant="outline"
      className={cn(
        "rounded-pill border-transparent text-2xs font-normal",
        STATUS[status],
      )}
    >
      {t(`mcp.recipeStatus.${status}`)}
    </Badge>
  );
}

export function ConfigRequirementBadges({
  requirements,
}: {
  requirements: ConfigRequirement[];
}) {
  const { t } = useTranslation();
  if (requirements.length === 0) return null;

  return (
    <>
      {requirements.map((requirement) => (
        <Badge
          key={requirement}
          variant="outline"
          className="rounded-pill border-transparent bg-muted text-2xs font-normal text-muted-foreground"
        >
          {t(`mcp.configRequirement.${requirement}`)}
        </Badge>
      ))}
    </>
  );
}

const AUTH_MODE_TONE: Record<AuthMode["type"], string> = {
  none: "bg-success-surface text-success",
  oauth2: "bg-primary/10 text-primary",
  bearer: "bg-warning-surface text-warning",
  basic: "bg-warning-surface text-warning",
  headers: "bg-warning-surface text-warning",
  env: "bg-muted text-muted-foreground",
  config_file: "bg-muted text-muted-foreground",
  path: "bg-muted text-muted-foreground",
  dsn: "bg-warning-surface text-warning",
};

export function AuthModeBadges({ modes }: { modes: AuthMode[] }) {
  const { t } = useTranslation();
  if (modes.length === 0) return null;

  return (
    <>
      {modes.map((mode) => (
        <Badge
          key={`${mode.type}:${mode.principal}:${mode.label ?? ""}`}
          variant="outline"
          title={mode.note}
          className={cn(
            "rounded-pill border-transparent text-2xs font-normal",
            AUTH_MODE_TONE[mode.type],
          )}
        >
          {mode.label ?? t(`mcp.authMode.${mode.type}`)}
        </Badge>
      ))}
    </>
  );
}
