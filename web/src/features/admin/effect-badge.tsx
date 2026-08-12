import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { Effect } from "@/features/admin/api";

// The pill shape the design system reserves for an outcome, in the status
// colour on its matching surface. Never colour alone: the word is the message.
const EFFECTS: Record<Effect, { label: string; className: string }> = {
  read: { label: "leitura", className: "bg-muted text-muted-foreground" },
  write: { label: "escrita", className: "bg-warning-surface text-warning" },
  destructive: {
    label: "destrutivo",
    className: "bg-danger-surface text-danger",
  },
  financial: {
    label: "financeiro",
    className: "bg-danger-surface text-danger",
  },
};

export function EffectBadge({ effect }: { effect: Effect }) {
  const spec = EFFECTS[effect];
  return (
    <Badge
      variant="outline"
      className={cn(
        "rounded-pill border-transparent font-mono text-2xs font-normal",
        spec.className,
      )}
    >
      {spec.label}
    </Badge>
  );
}
