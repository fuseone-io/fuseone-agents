import { useTranslation } from "react-i18next";
import { LayoutGrid, List } from "lucide-react";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

/** How the fleet is drawn. */
export type AgentsView = "cards" | "list";

/**
 * Cards or a list.
 *
 * A card answers "how is this one doing" at a glance and a list answers "which
 * one am I looking for" — the same fleet read two ways, and which one is
 * useful depends on whether there are six agents or sixty.
 *
 * A view toggle is not the screen's action, so it belongs beside the content
 * it changes rather than up in the header where the primary action lives. It
 * sits at the far end of the filter row: a reader narrowing the set moves left
 * to right, and choosing a shape is the last thing they do.
 */
export function AgentsViewToggle({
  view,
  onChange,
}: {
  view: AgentsView;
  onChange: (view: AgentsView) => void;
}) {
  const { t } = useTranslation();

  return (
    <ToggleGroup
      type="single"
      size="sm"
      value={view}
      // Never empty: pressing the active one again would leave the fleet drawn
      // as nothing, and a control that can turn the content off is a control
      // somebody presses once.
      onValueChange={(next) => next && onChange(next as AgentsView)}
      variant="outline"
      aria-label={t("agents.howToShow")}
    >
      <ToggleGroupItem value="cards" aria-label={t("agents.asCards")}>
        <LayoutGrid className="size-4" aria-hidden />
      </ToggleGroupItem>
      <ToggleGroupItem value="list" aria-label={t("agents.asList")}>
        <List className="size-4" aria-hidden />
      </ToggleGroupItem>
    </ToggleGroup>
  );
}
