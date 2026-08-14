import { useTranslation } from "react-i18next";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import {
  KINDS,
  labelOf,
  type BlockKind,
} from "@/features/agents/instruction-blocks";

/**
 * The label in the margin, and the way to change it.
 *
 * A label rather than a field, and still something somebody decides: an
 * instruction written before this editor existed arrives as one unlabelled
 * paragraph, and the only way to get it to the structure the screen offers is
 * to be able to say what each part is.
 *
 * "No label" stays on the list. A block that is simply prose is a real answer
 * — most instructions ever written are exactly that — and removing the option
 * would make the structure compulsory rather than available.
 */
export function BlockLabel({
  kind,
  onChange,
}: {
  kind: BlockKind;
  onChange: (kind: BlockKind) => void;
}) {
  const { t, i18n } = useTranslation();
  const label = labelOf(kind, i18n.language) || t("agents.blockProse");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn(
          "pt-[3px] text-right text-[10px]/5 font-medium uppercase tracking-label transition-colors hover:text-foreground",
          kind === "never" ? "text-danger" : "text-muted-foreground",
        )}
      >
        {label}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[220px]">
        {KINDS.map((one) => (
          <DropdownMenuItem key={one} onSelect={() => onChange(one)}>
            {labelOf(one, i18n.language)}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => onChange("prose")}>
          {t("agents.noLabel")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
