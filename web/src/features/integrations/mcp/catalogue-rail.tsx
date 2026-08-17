import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { CONNECTED_SHELF } from "@/features/integrations/mcp/catalogue";
import { cn } from "@/lib/utils";

/**
 * The shelves, with what is on each.
 *
 * A count beside a name is the difference between a filter somebody tries and
 * one they use: it says in advance whether clicking is worth it, and an empty
 * shelf says so without being clicked.
 *
 * The note underneath is the one thing this rail must never imply. Nothing
 * here is approved, homologated or supported by this platform — these are
 * servers other people publish, and what we ship is what we read about them.
 */
export function CatalogueRail({
  counts,
  connected,
  chosen,
  onChoose,
}: {
  counts: { category: string; count: number }[];
  connected: number;
  chosen: string | null;
  onChoose: (category: string | null) => void;
}) {
  const { t } = useTranslation();
  const total = counts.reduce((sum, one) => sum + one.count, 0);

  return (
    <nav className="flex shrink-0 flex-col gap-1 border-r bg-card p-3 sm:w-[196px]">
      <span className="px-2 pb-1 text-2xs uppercase tracking-label text-muted-foreground">
        {t("mcp.catalogueRailTitle")}
      </span>
      <Shelf
        label={t("mcp.everything")}
        count={total}
        active={chosen === null}
        onClick={() => onChoose(null)}
      />
      <Shelf
        label={t("mcp.connectedShelf")}
        count={connected}
        active={chosen === CONNECTED_SHELF}
        onClick={() => onChoose(CONNECTED_SHELF)}
      />
      {counts.map(({ category, count }) => (
        <Shelf
          key={category}
          label={t(`mcp.category.${category}`)}
          count={count}
          active={chosen === category}
          onClick={() => onChoose(category)}
        />
      ))}
      <p className="mt-auto border-t border-border-subtle px-2 pt-3 text-2xs leading-relaxed text-muted-foreground">
        {t("mcp.railCaveat")}
      </p>
    </nav>
  );
}

function Shelf({
  label,
  count,
  active,
  onClick,
}: {
  label: string;
  count: number;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={onClick}
      aria-current={active ? "true" : undefined}
      className={cn("justify-between", active && "bg-muted font-medium")}
    >
      {label}
      <span className="tabular-nums text-muted-foreground">{count}</span>
    </Button>
  );
}
