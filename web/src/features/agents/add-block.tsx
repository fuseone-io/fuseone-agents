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
import { KINDS, labelOf } from "@/features/agents/instruction-blocks";

/** The kinds a block can be. Labels, never a required schema. */
export function AddBlock({
  onAdd,
  locale,
}: {
  onAdd: (kind: (typeof KINDS)[number]) => void;
  locale: string;
}) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-[104px_minmax(0,68ch)] gap-x-5">
      <span />
      <DropdownMenu>
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
          {KINDS.map((kind) => (
            <DropdownMenuItem key={kind} onSelect={() => onAdd(kind)}>
              {labelOf(kind, locale)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
