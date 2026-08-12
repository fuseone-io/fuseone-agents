import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type TrailView = "list" | "diagram";

const OPTIONS: { value: TrailView; label: string }[] = [
  { value: "list", label: "Lista" },
  { value: "diagram", label: "Diagrama" },
];

/**
 * Which way to read the same run.
 *
 * A view toggle, not the screen's action: it belongs beside the content it
 * changes rather than up in the header. Neither view is a summary of the other
 * — they are the same steps under the same filter.
 */
export function TrailViewToggle({
  value,
  onChange,
}: {
  value: TrailView;
  onChange: (value: TrailView) => void;
}) {
  return (
    <div role="radiogroup" aria-label="Como ler a trilha" className="flex items-center gap-1.5">
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
