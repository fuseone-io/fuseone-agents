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

export function EffectBadge({ effect }: { effect: ToolEffect }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant="outline"
      className={cn(
        "rounded-pill border-transparent font-mono text-2xs font-normal",
        EFFECTS[effect],
      )}
    >
      {t(`effect.${effect}`)}
    </Badge>
  );
}
