import type { UseFormReturn } from "react-hook-form";
import type { StandingValues } from "@/features/admin/standing-form";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";

/**
 * Where a standing approval holds: a registered company, and one of its
 * areas or all of them. The form opens on the scope the console is reading.
 */
export function StandingScopeFields({
  form,
}: {
  form: UseFormReturn<StandingValues>;
}) {
  const [company, area] = form.watch(["company", "area"]);
  const { errors } = form.formState;
  return (
    <CompanyAreaFields
      value={{ company, area }}
      shape={{
        idPrefix: "standing",
        errors: { company: errors.company?.message, area: errors.area?.message },
      }}
      onChange={(next) => {
        form.setValue("company", next.company, { shouldValidate: true });
        form.setValue("area", next.area, { shouldValidate: true });
      }}
    />
  );
}
