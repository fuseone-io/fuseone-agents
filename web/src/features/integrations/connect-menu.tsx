import { useTranslation } from "react-i18next";
import { Plug, Plus, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

/**
 * The screen's one action, naming what it connects.
 *
 * "Conectar sistema" alone is ambiguous where there are two kinds, and a
 * header button that duplicated one section's own would leave the other
 * reachable only by scrolling.
 */
export function ConnectMenu({
  onConnect,
}: {
  onConnect: (kind: "server" | "provider") => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm">
          <Plus className="size-4" aria-hidden />
          {t("integrations.connect")}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => onConnect("server")}>
          <Server className="size-4" aria-hidden />
          {t("integrations.toolServer")}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onConnect("provider")}>
          <Plug className="size-4" aria-hidden />
          {t("integrations.modelProvider")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
