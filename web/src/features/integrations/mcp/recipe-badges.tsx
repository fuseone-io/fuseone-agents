import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

type RecipeStatus = ServerRecipe["status"];
type ConfigRequirement = ServerRecipe["configRequirements"][number];

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
