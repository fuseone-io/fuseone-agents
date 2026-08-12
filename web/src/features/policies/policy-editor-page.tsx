import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { IdentitySection } from "@/features/policies/identity-section";
import { ScopeSection } from "@/features/policies/scope-section";
import { ConditionSection } from "@/features/policies/condition-section";
import { EffectSection } from "@/features/policies/effect-section";
import { PolicySideRail } from "@/features/policies/policy-side-rail";
import { usePolicyDraft } from "@/features/policies/policy-form";
import { usePolicies, usePutPolicy } from "@/features/policies/api";
import { useCode } from "@/features/policies/use-code";

/**
 * One rule, written or rewritten.
 *
 * Creating and editing are the same screen because they are the same act: the
 * code is the identity, and everything else is what the rule says this time.
 */
export function PolicyEditorPage() {
  const { code: routeCode } = useParams();
  const creating = routeCode === undefined || routeCode === "new";
  const navigate = useNavigate();

  const { data, isLoading, error, refetch } = usePolicies();
  const loaded = data?.items.find((p) => p.code === routeCode);
  const { code, setCode } = useCode(creating, data?.items ?? [], routeCode);
  const { draft, patch, changes } = usePolicyDraft(loaded);
  const save = usePutPolicy();

  if (isLoading) return <LoadingRows rows={6} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;
  if (!creating && !loaded) {
    return <ErrorState error={new Error(`Nenhuma política com o código ${routeCode}`)} />;
  }

  const submit = () => {
    save.mutate(
      { code, policy: draft },
      {
        onSuccess: () => {
          toast.success(creating ? `${code} criada` : `${code} salva`, {
            description:
              draft.mode === "monitor"
                ? "Em modo monitorar: avaliada, registrada, sem mudar nada."
                : "Impondo a partir do próximo passo de agente.",
          });
          navigate("/policies");
        },
        onError: (e) =>
          toast.error("Não foi possível gravar", {
            description: e instanceof Error ? e.message : undefined,
          }),
      },
    );
  };

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.policies}
        title={creating ? "Nova política" : loaded!.name}
        description="Uma regra avaliada em todo passo de agente: escopo, condição, e o que acontece quando bate."
      />

      <div className="grid gap-5 lg:grid-cols-[1fr_316px] lg:items-start">
        <div className="flex flex-col gap-4">
          <IdentitySection draft={draft} patch={patch} code={code} editable={creating} onCode={setCode} />
          <ScopeSection draft={draft} patch={patch} />
          <ConditionSection draft={draft} patch={patch} />
          <EffectSection draft={draft} patch={patch} />
        </div>

        <PolicySideRail draft={draft} creating={creating} changes={changes} />
      </div>

      {/* The commit never leaves the screen, and its label names the
          consequence: a rule that will watch and a rule that will stop things
          are different acts and must not share a button that says "Salvar". */}
      <div className="sticky bottom-0 -mx-6 -mb-6 flex items-center gap-2 border-t border-border bg-card px-6 py-3 shadow-md">
        {changes.length > 0 && (
          <span className="text-xs text-warning">
            {changes.length} {changes.length === 1 ? "alteração" : "alterações"} sem gravar
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" onClick={() => navigate("/policies")}>
            Cancelar
          </Button>
          <Button onClick={submit} disabled={save.isPending || !draft.name || !code}>
            {draft.mode === "monitor" ? "Gravar em modo monitorar" : "Gravar e impor"}
          </Button>
        </div>
      </div>
    </>
  );
}
