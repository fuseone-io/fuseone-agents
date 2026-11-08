import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { StaleNotice } from "@/components/shared/stale-notice";
import { IdentitySection } from "@/features/policies/identity-section";
import { ReachSection } from "@/features/policies/reach-section";
import { ScopeSection } from "@/features/policies/scope-section";
import { ConditionSection } from "@/features/policies/condition-section";
import { EffectSection } from "@/features/policies/effect-section";
import { PolicySideRail } from "@/features/policies/policy-side-rail";
import { usePolicyDraft } from "@/features/policies/policy-form";
import { usePolicies, usePolicy, usePutPolicy } from "@/features/policies/api";
import { useCode } from "@/features/policies/use-code";
import { ApiError, type Policy, type PolicyInput } from "@/lib/api/client";
import { isStale, type FormRevision } from "@/lib/api/revision";

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

  // The list is for the code a new rule may take; the rule itself is read by
  // its code, with the revision the save answers.
  const { data, isLoading, error, refetch } = usePolicies();
  const read = usePolicy(creating ? undefined : routeCode);
  const loaded = read.data?.body;

  // Only a read made as the editor opened seeds it, never a cached copy.
  if (isLoading || (!creating && !read.isFetchedAfterMount)) {
    return <LoadingRows rows={6} />;
  }
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;
  // Gone is no such rule, before any copy the last opening cached.
  const gone = read.error instanceof ApiError && read.error.status === 404;
  if (read.error && !gone) {
    return (
      <ErrorState error={read.error} onRetry={() => void read.refetch()} />
    );
  }
  if (!creating && (gone || !loaded)) {
    return (
      <ErrorState
        error={new Error(t("policies.noneWithCode", { code: routeCode }))}
      />
    );
  }

  return (
    <PolicyForm
      // A new revision is a new form: reloading replaces the draft with it.
      key={creating ? "new" : `${loaded!.code}:${read.data!.etag}`}
      creating={creating}
      loaded={loaded}
      policies={data?.items ?? []}
      routeCode={routeCode}
      revision={
        read.data && {
          etag: read.data.etag,
          reload: () => void read.refetch(),
          reloading: read.isFetching,
        }
      }
    />
  );
}

function PolicyForm({
  creating,
  loaded,
  policies,
  routeCode,
  revision,
}: {
  creating: boolean;
  loaded?: Policy;
  policies: Policy[];
  routeCode?: string;
  revision?: FormRevision;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { code, setCode } = useCode(creating, policies, routeCode);
  const { form, draft, patch, valid, changes } = usePolicyDraft(loaded);
  const save = usePutPolicy();
  const [stale, setStale] = useState(false);

  const submit = (policy: PolicyInput) => {
    save.mutate(
      { code, policy, etag: revision?.etag },
      {
        onSuccess: () => {
          toast.success(creating ? `${code} criada` : `${code} salva`, {
            description:
              policy.mode === "monitor"
                ? t("policies.monitorMode")
                : t("policies.enforcingNextStep"),
          });
          navigate("/policies");
        },
        // Refused because somebody wrote first: the draft stays, and the
        // notice offers the stored version instead of a toast that fades.
        onError: (e) =>
          isStale(e)
            ? setStale(true)
            : toast.error(t("policies.saveFailed"), {
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
          onClick={form.handleSubmit(submit)}
          disabled={save.isPending || !code || !valid}
        >
          {draft.mode === "monitor"
            ? t("policies.saveMonitoring")
            : t("policies.saveAndEnforce")}
        </Button>
      </PageHeader>

      <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_316px] lg:items-start">
        <div className="flex min-w-0 flex-col gap-4">
          {stale && revision && (
            <StaleNotice
              onReload={revision.reload}
              reloading={revision.reloading}
            />
          )}
          <IdentitySection
            draft={draft}
            patch={patch}
            code={code}
            editable={creating}
            onCode={setCode}
          />
          <ScopeSection draft={draft} patch={patch} />
          <ReachSection draft={draft} patch={patch} />
          <ConditionSection draft={draft} patch={patch} />
          <EffectSection draft={draft} patch={patch} />
        </div>

        <PolicySideRail draft={draft} creating={creating} changes={changes} />
      </div>
    </div>
  );
}
