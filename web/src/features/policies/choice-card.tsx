import { useId } from "react";
import { Label } from "@/components/ui/label";
import { RadioGroupItem } from "@/components/ui/radio-group";
import { cn } from "@/lib/utils";

/**
 * One option of a policy radio group, drawn as a card the whole of which is
 * clickable.
 *
 * The radio itself is the shadcn `RadioGroupItem`, so the group keeps Radix's
 * roving focus, arrow keys and `aria-checked`; the card is its `Label`, which
 * is what gives the radio its accessible name. Styling for the chosen state is
 * read from the radio through `has-data-[state=checked]`, so there is no second
 * copy of "which one is chosen" to drift from the group's value.
 */
export function ChoiceCard({
  value,
  label,
  note,
  className,
}: {
  value: string;
  label: string;
  note?: string;
  /** Layout plus the `has-data-[state=checked]:` look of the chosen card. */
  className?: string;
}) {
  const id = useId();
  return (
    <Label
      htmlFor={id}
      className={cn(
        "min-w-0 cursor-pointer items-start rounded-lg border border-border font-normal",
        className,
      )}
    >
      <RadioGroupItem value={value} id={id} className="mt-0.5" />
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="break-words text-sm font-medium">{label}</span>
        {note && (
          <span className="break-words text-xs text-muted-foreground">
            {note}
          </span>
        )}
      </span>
    </Label>
  );
}
