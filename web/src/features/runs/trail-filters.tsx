import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { TrailFilter } from "@/features/runs/trail-model";

const OPTIONS: { value: TrailFilter; label: string }[] = [
  { value: "all", label: "Tudo" },
  { value: "tools", label: "Ferramentas" },
  { value: "policy", label: "Política" },
];

/**
 * What the reader wants to see of the trail.
 *
 * A radio group rather than a set of toggles: the three views are answers to
 * one question, and a reader who could select none of them would be looking at
 * an empty audit trail with no way to tell why.
 */
export function TrailFilters({
  value,
  onChange,
}: {
  value: TrailFilter;
  onChange: (value: TrailFilter) => void;
}) {
  return (
    <div role="radiogroup" aria-label="Filtrar a trilha" className="flex items-center gap-1.5">
      {OPTIONS.map((option) => (
        <Button
          key={option.value}
          role="radio"
          aria-checked={value === option.value}
          variant="outline"
          size="sm"
          className={cn(
            "h-[26px] rounded-pill px-2.5 text-xs font-normal",
            value === option.value ? "text-foreground" : "text-muted-foreground",
          )}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}
