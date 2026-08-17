import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Layers, Plus } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { LoadMore } from "@/components/shared/load-more";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { RemoveButton } from "@/components/shared/remove-button";
import { AreaForm } from "@/features/admin/area-form";
import {
  useDeleteScope,
  useScopes,
  type RegisteredScope,
} from "@/features/scope/api";
import { useVisibleItems } from "@/hooks/use-visible-items";

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
  const [adding, setAdding] = useState(false);
  const areas = data?.items ?? [];
  const page = useVisibleItems(areas, 50);

  return (
    <Panel
      title={t("admin.areas")}
      action={
        <Button size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-4" />
          {t("admin.newFeminine")}
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : areas.length === 0 ? (
        <EmptyState
          icon={<Layers className="size-6" />}
          title={t("scope.noAreas")}
          hint={t("admin.areaHint")}
        />
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {page.visible.map((area) => (
              <AreaRow key={`${area.company}/${area.area}`} area={area} />
            ))}
          </ul>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}

      {adding && <AreaForm onClose={() => setAdding(false)} />}
    </Panel>
  );
}

function AreaRow({ area }: { area: RegisteredScope }) {
  const { t } = useTranslation();
  const remove = useDeleteScope();
  const shown = area.label || area.area;

  return (
    <li className="flex items-center gap-2 rounded-lg border p-3">
      <div className="min-w-0 flex-1">
        <div className="font-medium">{shown}</div>
        {/* The id, always, and never only the label: it is what a ceiling, a
            policy and an agent all reference, and what somebody types into a
            file. A row showing only "Atendimento" hides that it is `cx`. */}
        <Mono dim>
          {area.company}/{area.area}
        </Mono>
      </div>
      <RemoveButton
        title={t("admin.removeArea", { area: shown })}
        description={t("admin.withdrawArea")}
        onConfirm={() =>
          remove.mutate(`${area.company}/${area.area}`, {
            onSuccess: () => toast.success(t("admin.areaWithdrawn")),
            onError: (e) =>
              toast.error(
                e instanceof Error ? e.message : t("admin.withdrawFailed"),
              ),
          })
        }
      />
    </li>
  );
}
