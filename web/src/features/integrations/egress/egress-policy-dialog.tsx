import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel } from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { problemMessage } from "@/lib/api/problem-message";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";
import { ScopeListsNotice } from "@/features/scope/scope-lists-notice";
import { originsOf, policyAt, scopeLabel, usePutEgressPolicy, type EgressPolicy } from "@/features/integrations/egress/api";
import { isStale } from "@/lib/api/revision";

const schema = z.object({
  scope: z.object({ company: z.string().min(1, "integrations.egress.companyRequired"), area: z.string() }),
  origins: z.string(),
});
type PolicyValues = z.infer<typeof schema>;

/**
 * Sets the origins a company's or an area's MCP servers may reach. A policy
 * read for editing keeps its scope, which is its identity; a new one chooses
 * it from what is registered. The server judges each origin.
 *
 * A policy is replaced whole. So a new one for a scope that already has a
 * policy opens that policy, with its origins and the revision they were read
 * at, rather than replacing it with what was typed: that is how a company's
 * servers once lost every origin but the one somebody meant to add.
 */
export function EgressPolicyDialog({
  policy,
  policies,
  onClose,
}: {
  policy: EgressPolicy | null;
  policies: EgressPolicy[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutEgressPolicy();
  const [refusal, setRefusal] = useState<string>();
  // The policy this write edits: the one opened, or the one the scope chosen has.
  const [found, setFound] = useState<EgressPolicy | undefined>(undefined);
  const editing = policy ?? found;
  const form = useForm<PolicyValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      scope: { company: policy?.company ?? "", area: policy?.area ?? "" },
      origins: (policy?.origins ?? []).join("\n"),
    },
  });

  async function submit(values: PolicyValues) {
    setRefusal(undefined);
    try {
      await put.mutateAsync({ ...values.scope, origins: originsOf(values.origins), etag: editing?.etag });
      toast.success(t("integrations.egress.saved", { scope: scopeLabel(values.scope) }));
      onClose();
    } catch (problem) {
      setRefusal(isStale(problem) ? t("integrations.egress.stale") : problemMessage(problem, t));
    }
  }

  // A new policy for a scope that has one becomes an edit of it, keeping what
  // was typed beside what is stored.
  // Moving on to another scope takes the loaded origins back out, so they are
  // never written to a scope they were not read from.
  function chooseScope(scope: PolicyValues["scope"]) {
    const stored = policy ? undefined : policyAt(policies, scope);
    const typed = originsOf(form.getValues("origins")).filter((origin) => !found?.origins.includes(origin));
    setFound(stored);
    if (stored || found) {
      const kept = stored ? [...stored.origins, ...typed.filter((origin) => !stored.origins.includes(origin))] : typed;
      form.setValue("origins", kept.join("\n"));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{policy ? t("integrations.egress.editTitle", { scope: scopeLabel(policy) }) : t("integrations.egress.newTitle")}</DialogTitle>
          <DialogDescription>{t("integrations.egress.dialogHint")}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form id="egress-policy" onSubmit={form.handleSubmit(submit)} className="space-y-4">
            <FormField
              control={form.control}
              name="scope"
              render={({ field }) => (
                <div className="grid gap-3 sm:grid-cols-2">
                  <CompanyAreaFields
                    value={field.value}
                    disabled={policy !== null}
                    shape={{ idPrefix: "egress", errors: { company: form.formState.errors.scope?.company?.message } }}
                    onChange={(scope) => {
                      field.onChange(scope);
                      chooseScope(scope);
                    }}
                  />
                </div>
              )}
            />
            <ScopeListsNotice message={t("scope.listsFailed")} />
            {found && (
              <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
                {t("integrations.egress.scopeHasPolicy", { scope: scopeLabel(found) })}
              </p>
            )}
            <FormField
              control={form.control}
              name="origins"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.egress.origins")}</FormLabel>
                  <FormControl>
                    <Textarea rows={6} className="font-mono text-xs" {...field} />
                  </FormControl>
                  <FormDescription>{t("integrations.egress.originsHint")}</FormDescription>
                </FormItem>
              )}
            />
            {refusal && <p role="alert" className="text-sm text-danger">{refusal}</p>}
          </form>
        </Form>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" form="egress-policy" disabled={put.isPending}>{t("common.save")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
