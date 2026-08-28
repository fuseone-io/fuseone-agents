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
import type { Policy } from "@/lib/api/client";

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

  const { data, isLoading, error, refetch } = usePolicies();
  const loaded = data?.items.find((p) => p.code === routeCode);

  if (isLoading) return <LoadingRows rows={6} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;
  if (!creating && !loaded) {
    return (
      <ErrorState
        error={new Error(t("policies.noneWithCode", { code: routeCode }))}
      />
    );
  }

  return (
    <PolicyForm
      key={creating ? "new" : loaded!.code}
      creating={creating}
      loaded={loaded}
      policies={data?.items ?? []}
      routeCode={routeCode}
    />
  );
}

function PolicyForm({
  creating,
  loaded,
  policies,
  routeCode,
}: {
  creating: boolean;
  loaded?: Policy;
  policies: Policy[];
  routeCode?: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { code, setCode } = useCode(creating, policies, routeCode);
  const { draft, patch, changes } = usePolicyDraft(loaded);
  const save = usePutPolicy();

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
    // No overflow-x here, deliberately. Setting one axis to anything but
    // visible makes CSS use-value the other axis to auto, so overflow-x-clip
    // gave this page an implicit overflow-y: auto — a second scroll container
    // inside the shell's, which is why creating a policy showed two bars and
    // no other screen did. Containment is min-w-0 and max-w-full, which is what
    // was actually holding the width.
    <div className="flex min-w-0 max-w-full flex-col gap-6">
      <PageHeader
        icon={PAGE_ICONS.policies}
        title={creating ? t("policies.newPolicy") : loaded!.name}
        description={t("policies.editorSubtitle")}
      >
        {changes.length > 0 && (
          <span className="min-w-0 text-xs text-warning">
            {t("policies.unsavedChanges", { count: changes.length })}
          </span>
        )}
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
      </PageHeader>

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
    </div>
  );
}
