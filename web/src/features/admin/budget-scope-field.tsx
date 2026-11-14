import { useTranslation } from "react-i18next";
import type { Control } from "react-hook-form";
import { FormField } from "@/components/ui/form";
import type { BudgetValues } from "@/features/admin/budget-form";
import { pathOfScope, scopeOfPath } from "@/features/admin/budget-scope";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";

/**
 * What a ceiling applies to: the installation, a company, or one of its
 * areas, chosen from what is registered. The form keeps the path the API
 * reads — `installation`, `acme` or `acme/cx` — so what it sends is the same
 * whichever way it was chosen. A ceiling's scope is its identity: editing one
 * only shows it.
 */
export function BudgetScopeField({
  control,
  locked,
}: {
  control: Control<BudgetValues>;
  locked: boolean;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name="scope"
      render={({ field, fieldState }) => (
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">{t("admin.appliesTo")}</legend>
          <div className="grid gap-3 sm:grid-cols-2">
            <CompanyAreaFields
              value={scopeOfPath(field.value)}
              shape={{
                idPrefix: "ceiling",
                installation: true,
                errors: { company: fieldState.error?.message },
              }}
              disabled={locked}
              onChange={(next) => field.onChange(pathOfScope(next))}
            />
          </div>
        </fieldset>
      )}
    />
  );
}
