import { Check, Layers } from "lucide-react";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

/** One context in the switcher. The tick, not the highlight, says which is
 *  current: the highlight is where the pointer is. */
export function ScopeChoice({
  label,
  chosen,
  indented,
  onChoose,
}: {
  label: string;
  chosen: boolean;
  indented?: boolean;
  onChoose: () => void;
}) {
  return (
    <DropdownMenuItem onSelect={onChoose} className={cn("gap-2", indented && "pl-8")}>
      {!indented && <Layers className="size-3.5 shrink-0 opacity-60" aria-hidden />}
      <span className="truncate">{label}</span>
      {chosen && <Check className="ml-auto size-4 shrink-0 text-primary" aria-hidden />}
    </DropdownMenuItem>
  );
}
