import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ScopeValues } from "@/features/integrations/scope/scope-choice";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";

/** Any registered scope: for the platform's administration. */
export function FreeScopeFields({ editing, label }: { editing: boolean; label: string }) {
  const { t } = useTranslation();
  const form = useFormContext<ScopeValues>();
  const scopeKind = form.watch("scopeKind");
  return (
    <>
      <FormField
        control={form.control}
        name="scopeKind"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t(label)}</FormLabel>
            <Select
              value={field.value}
              onValueChange={field.onChange}
              disabled={editing}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="area">
                  {t("connectors.scope.area")}
                </SelectItem>
                <SelectItem value="company">
                  {t("connectors.scope.company")}
                </SelectItem>
                <SelectItem value="installation">
                  {t("connectors.scope.installation")}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      {scopeKind !== "installation" && <PlacedScope editing={editing} />}
    </>
  );
}

/** The company, and the area when the row lives at one. */
function PlacedScope({ editing }: { editing: boolean }) {
  const form = useFormContext<ScopeValues>();
  const [scopeKind, company, area] = form.watch(["scopeKind", "company", "area"]);
  const { errors, isSubmitted } = form.formState;
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <CompanyAreaFields
        value={{ company, area }}
        shape={{
          idPrefix: "scope",
          area: scopeKind === "area" ? "required" : "none",
          errors: { company: errors.company?.message, area: errors.area?.message },
        }}
        disabled={editing}
        onChange={(next) => {
          form.setValue("company", next.company, { shouldValidate: isSubmitted });
          form.setValue("area", next.area, { shouldValidate: isSubmitted });
        }}
      />
    </div>
  );
}
