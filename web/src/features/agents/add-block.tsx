import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Ban,
  Hand,
  ListChecks,
  MessageSquareQuote,
  Target,
  Wrench,
} from "lucide-react";
import { DropdownMenuSeparator } from "@/components/ui/dropdown-menu";
import { Mono } from "@/components/shared/mono";
import { KINDS, labelOf } from "@/features/agents/instruction-blocks";

/** The icon each kind carries, so the menu is scannable rather than read. */
const ICONS = {
  objective: Target,
  howToAct: ListChecks,
  whenToStop: Hand,
  never: Ban,
  howToReply: MessageSquareQuote,
};

/** The kinds a block can be. Labels, never a required schema. */
export function AddBlock({
  open,
  onOpenChange,
  onAdd,
  onCite,
  locale,
}: {
  /** Open because somebody typed `/`, rather than pressed the button. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  onAdd: (kind: (typeof KINDS)[number]) => void;
  /** Citing a tool, which is the other thing somebody comes here to do. */
  onCite: () => void;
  locale: string;
}) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-[104px_minmax(0,68ch)] gap-x-5">
      <span />
      <DropdownMenu open={open} onOpenChange={onOpenChange}>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className="h-7 justify-start border-dashed text-2xs text-muted-foreground"
          >
            <Plus className="size-3.5" aria-hidden />
            {t("agents.newBlock")}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-[300px]">
          <DropdownMenuLabel className="text-2xs uppercase tracking-label text-muted-foreground">
            {t("agents.blocks")}
          </DropdownMenuLabel>
          {KINDS.map((kind) => {
            const Icon = ICONS[kind];
            return (
              <DropdownMenuItem
                key={kind}
                onSelect={() => onAdd(kind)}
                className="h-8 gap-2"
              >
                <Icon className="size-3.5" aria-hidden />
                <span className="flex-1">{labelOf(kind, locale)}</span>
                <span className="text-2xs text-muted-foreground">
                  {t(`agents.blockHint.${kind}` as const)}
                </span>
              </DropdownMenuItem>
            );
          })}

          {/* The other way to name something, offered where somebody is
              already looking for a way to add. */}
          <DropdownMenuSeparator />
          <DropdownMenuItem className="h-8 gap-2" onSelect={onCite}>
            <Wrench className="size-3.5" aria-hidden />
            <span className="flex-1">{t("agents.citeAToolItem")}</span>
            <Mono dim className="text-2xs">
              @
            </Mono>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
