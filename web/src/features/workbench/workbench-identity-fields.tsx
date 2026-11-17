import { useTranslation } from "react-i18next";
import { useFormContext, useWatch, type Control } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AgentAreaField } from "@/features/agents/agent-area-field";

export interface IdentityValues {
  agentId: string;
  name: string;
  company: string;
  area: string;
}

/** The identifier, the name and the scope a new agent is born with. */
export function WorkbenchIdentityFields({ control }: { control: Control<IdentityValues> }) {
  const { t } = useTranslation();
  const { setValue } = useFormContext<IdentityValues>();
  const company = useWatch({ control, name: "company" });
  // The area is chosen with its company, among the declared ones.
  const choose = (scope: { company: string; area: string }) => {
    setValue("company", scope.company, { shouldValidate: true });
    setValue("area", scope.area, { shouldValidate: true });
  };

  return (
    <>
      <FormField
        control={control}
        name="agentId"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("agents.identifier")}</FormLabel>
            <FormControl>
              <Input {...field} className="font-mono" placeholder="suporte" />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.name")}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name="area"
        render={({ field }) => (
          <FormItem>
            <AgentAreaField company={company} area={field.value} onChange={choose} />
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
