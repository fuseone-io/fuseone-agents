import { useDeferredValue, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Layers } from "lucide-react";
import { toast } from "sonner";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { AreaForm } from "@/features/admin/area-form";
import {
  companyOptionsFor,
  matchesArea,
} from "@/features/admin/area-filters";
import { AreaHeader } from "@/features/admin/area-header";
import { AreaList } from "@/features/admin/area-list";
import { AreaToolbar } from "@/features/admin/area-toolbar";
import { useCompanies } from "@/features/companies/api";
import { useMe } from "@/features/session/api";
import {
  useDeleteScope,
  useScopes,
  type RegisteredScope,
} from "@/features/scope/api";
import { useVisibleItems } from "@/hooks/use-visible-items";

const EMPTY_AREAS: RegisteredScope[] = [];

/**
 * The areas work is filed under.
 *
 * Before this existed, an area was created by typing one into an agent, which
 * made two spellings two areas: a ceiling on one governed no agent under the
 * other and nothing reported it. Declaring them is also what lets somebody
 * switch context into an area before any agent lives there.
 */
export function AreasPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useScopes();
  const { data: me } = useMe();
  const canListCompanies = me === null || me?.can.includes("company:write");
  const companiesQuery = useCompanies({ enabled: Boolean(canListCompanies) });
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<RegisteredScope | null>(null);
  const [search, setSearch] = useState("");
  const [openRows, setOpenRows] = useState<Record<string, boolean>>({});
  const remove = useDeleteScope();

  const areas = data?.items ?? EMPTY_AREAS;
  const query = useDeferredValue(search.trim().toLowerCase());
  const filtered = useMemo(
    () => areas.filter((area) => !query || matchesArea(area, query, t)),
    [areas, query, t],
  );
  const page = useVisibleItems(filtered, 50);
  const companyOptions = companyOptionsFor({
    grants: me?.grants ?? [],
    companies: companiesQuery.data?.items ?? [],
    areas,
  });
  const companies = new Set(areas.map((area) => area.company)).size;
  const noMatches = areas.length > 0 && filtered.length === 0;

  return (
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <AreaHeader onAdd={() => setAdding(true)} />
      {isLoading ? (
        <div className="p-4">
          <LoadingRows rows={3} />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : areas.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Layers className="size-6" />}
            title={t("scope.noAreas")}
            hint={t("admin.areaHint")}
          />
        </div>
      ) : noMatches ? (
        <>
          <AreaToolbar
            search={search}
            onSearch={setSearch}
            total={areas.length}
            companies={companies}
          />
          <NoAreaMatches />
        </>
      ) : (
        <>
          <AreaToolbar
            search={search}
            onSearch={setSearch}
            total={areas.length}
            companies={companies}
          />
          <AreaList
            areas={page.visible}
            openRows={openRows}
            shown={page.loaded}
            total={areas.length}
            hasMore={page.hasMore}
            onOpenChange={(scope, open) =>
              setOpenRows((rows) => ({ ...rows, [scope]: open }))
            }
            onEdit={setEditing}
            onRemove={(area) =>
              remove.mutate(`${area.company}/${area.area}`, {
                onSuccess: () => toast.success(t("admin.areaWithdrawn")),
                onError: () => toast.error(t("admin.withdrawFailed")),
              })
            }
            onLoadMore={page.loadMore}
          />
        </>
      )}

      {adding && (
        <AreaForm
          companyOptions={companyOptions}
          onClose={() => setAdding(false)}
        />
      )}
      {editing && (
        <AreaForm
          area={editing}
          companyOptions={companyOptions}
          onClose={() => setEditing(null)}
        />
      )}
    </section>
  );
}

function NoAreaMatches() {
  const { t } = useTranslation();
  return (
    <div className="p-8">
      <EmptyState
        icon={<Layers className="size-6" />}
        title={t("admin.noAreasFound")}
        hint={t("admin.noAreasFoundHint")}
      />
    </div>
  );
}
