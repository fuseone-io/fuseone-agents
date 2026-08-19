import { AreaFooter } from "@/features/admin/area-footer";
import { AreaRow } from "@/features/admin/area-row";
import { AreaTableHeader } from "@/features/admin/area-table-header";
import type { RegisteredScope } from "@/features/scope/api";

export function AreaList({
  areas,
  openRows,
  shown,
  total,
  hasMore,
  onOpenChange,
  onEdit,
  onRemove,
  onLoadMore,
}: {
  areas: RegisteredScope[];
  openRows: Record<string, boolean>;
  shown: number;
  total: number;
  hasMore: boolean;
  onOpenChange: (scope: string, open: boolean) => void;
  onEdit: (area: RegisteredScope) => void;
  onRemove: (area: RegisteredScope) => void;
  onLoadMore: () => void;
}) {
  return (
    <>
      <ul className="divide-y divide-border-subtle">
        <AreaTableHeader />
        {areas.map((area) => {
          const key = `${area.company}/${area.area}`;
          return (
            <li key={key}>
              <AreaRow
                area={area}
                open={Boolean(openRows[key])}
                onOpenChange={(open) => onOpenChange(key, open)}
                onEdit={() => onEdit(area)}
                onRemove={() => onRemove(area)}
              />
            </li>
          );
        })}
      </ul>
      <AreaFooter
        shown={shown}
        total={total}
        hasMore={hasMore}
        onLoadMore={onLoadMore}
      />
    </>
  );
}
