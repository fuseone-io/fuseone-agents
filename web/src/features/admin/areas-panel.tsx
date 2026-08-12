import { useState } from "react";
import { Layers, Plus } from "lucide-react";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { RemoveButton } from "@/features/admin/remove-button";
import { AreaForm } from "@/features/admin/area-form";
import { useDeleteScope, useScopes, type RegisteredScope } from "@/features/scope/api";

/**
 * The areas work is filed under.
 *
 * Before this existed, an area was created by typing one into an agent, which
 * made two spellings two areas: a ceiling on one governed no agent under the
 * other and nothing reported it. Declaring them is also what lets somebody
 * switch context into an area before any agent lives there.
 */
export function AreasPanel() {
  const { data, isLoading, error, refetch } = useScopes();
  const [adding, setAdding] = useState(false);
  const areas = data?.items ?? [];

  return (
    <Panel
      title="Áreas"
      action={
        <Button size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-4" />
          Nova
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
          title="Nenhuma área declarada"
          hint="Uma área é onde o trabalho fica arquivado: o agente pertence a uma, o teto governa uma, a política alcança um conjunto delas e a concessão é feita em uma. Sem declarar, cada tela inventa a sua a partir do que existe."
        />
      ) : (
        <ul className="flex flex-col gap-2">
          {areas.map((area) => (
            <AreaRow key={`${area.company}/${area.area}`} area={area} />
          ))}
        </ul>
      )}

      {adding && <AreaForm onClose={() => setAdding(false)} />}
    </Panel>
  );
}

function AreaRow({ area }: { area: RegisteredScope }) {
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
        title={`Retirar a área ${shown}?`}
        description="Ela deixa de ser oferecida para trabalho novo. Os agentes, tetos e políticas já arquivados nela continuam apontando para ela e continuam valendo — retirar uma área e reescrever o passado são atos diferentes."
        onConfirm={() =>
          remove.mutate(`${area.company}/${area.area}`, {
            onSuccess: () => toast.success("Área retirada"),
            onError: (e) => toast.error(e instanceof Error ? e.message : "Não foi possível retirar"),
          })
        }
      />
    </li>
  );
}
