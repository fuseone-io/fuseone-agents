import { Button } from "@/components/ui/button";

export function PriceSuggestionButtons({
  label,
  options,
  disabled,
  onChoose,
}: {
  label: string;
  options: string[];
  disabled: boolean;
  onChoose: (value: string) => void;
}) {
  if (options.length === 0) return null;
  return (
    <div aria-label={label} className="flex flex-wrap gap-1 pt-1">
      <p className="w-full text-xs font-medium text-muted-foreground">
        {label}
      </p>
      {options.map((option) => (
        <Button
          key={option}
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled}
          className="h-7 max-w-full px-2 font-mono text-xs"
          onClick={() => onChoose(option)}
        >
          <span className="truncate">{option}</span>
        </Button>
      ))}
    </div>
  );
}
