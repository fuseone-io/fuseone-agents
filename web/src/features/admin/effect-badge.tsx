import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ToolEffect } from "@/features/admin/api";

/*
The pill shape the design system reserves for an outcome, in the status colour
on its matching surface. Never colour alone: the word is the message.

`unknown` is the state of a tool nobody has ruled on, and it is a refusal at
run time — so it wears the refusing colour rather than a neutral grey. A tool
the platform will stop, shown as though it were merely uneventful, is the
screen disagreeing with the Gate about the row somebody is deciding on.
*/
const EFFECTS: Record<ToolEffect, string> = {
  unknown: "bg-danger-surface text-danger",
  read: "bg-muted text-muted-foreground",
  write: "bg-warning-surface text-warning",
  destructive: "bg-danger-surface text-danger",
  financial: "bg-danger-surface text-danger",
};

/*
`stale` is a refusal too, and a different job.

A tool nobody ruled on is a decision to make. A tool whose ruling was overtaken
by a new definition is a decision to *check* — somebody already looked, and
what they looked at is not what is on offer now. Both are blocked by the Gate;
rendering them alike makes the second read as an oversight rather than as work
the platform is asking for.
*/
export function EffectBadge({
  effect,
  stale = false,
}: {
  effect: ToolEffect;
  stale?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Badge
      variant="outline"
      className={cn(
        "rounded-pill border-transparent font-mono text-2xs font-normal",
        stale ? "bg-danger-surface text-danger" : EFFECTS[effect],
      )}
    >
      {stale ? t("effect.stale") : t(`effect.${effect}`)}
    </Badge>
  );
}
