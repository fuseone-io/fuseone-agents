import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
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
    return (
      <ErrorState
        error={new Error(t("policies.noneWithCode", { code: routeCode }))}
      />
    );
  }

  const submit = () => {
    save.mutate(
      { code, policy: draft },
      {
        onSuccess: () => {
          toast.success(creating ? `${code} criada` : `${code} salva`, {
            description:
              draft.mode === "monitor"
                ? t("policies.monitorMode")
                : t("policies.enforcingNextStep"),
          });
          navigate("/policies");
        },
        onError: (e) =>
          toast.error(t("policies.saveFailed"), {
            description: e instanceof Error ? e.message : undefined,
          }),
      },
    );
  };

  return (
    <div className="flex min-w-0 max-w-full flex-col gap-6 overflow-x-clip">
      <PageHeader
        icon={PAGE_ICONS.policies}
        title={creating ? t("policies.newPolicy") : loaded!.name}
        description={t("policies.editorSubtitle")}
      />

      <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_316px] lg:items-start">
        <div className="flex min-w-0 flex-col gap-4">
          <IdentitySection
            draft={draft}
            patch={patch}
            code={code}
            editable={creating}
            onCode={setCode}
          />
          <ScopeSection draft={draft} patch={patch} />
          <ConditionSection draft={draft} patch={patch} />
          <EffectSection draft={draft} patch={patch} />
        </div>

        <PolicySideRail draft={draft} creating={creating} changes={changes} />
      </div>

      {/* The commit never leaves the screen, and its label names the
          consequence: a rule that will watch and a rule that will stop things
          are different acts and must not share a button that says t("common.save"). */}
      <div className="sticky bottom-0 z-10 mt-4 flex min-w-0 flex-wrap items-center gap-2 rounded-t-xl border border-border bg-card px-4 py-3 shadow-md sm:px-6">
        {changes.length > 0 && (
          <span className="min-w-0 text-xs text-warning">
            {t("policies.unsavedChanges", { count: changes.length })}
          </span>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Button variant="outline" onClick={() => navigate("/policies")}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={save.isPending || !draft.name || !code}
          >
            {draft.mode === "monitor"
              ? t("policies.saveMonitoring")
              : t("policies.saveAndEnforce")}
          </Button>
        </div>
      </div>
    </div>
  );
}
