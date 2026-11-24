import type { UseFormReturn } from "react-hook-form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import { CustomHeaderFields } from "@/features/integrations/mcp/custom-header-fields";

/**
 * The custom header inside the connection form. The schema owns the values and
 * their validation; this only adapts them to the shared fields.
 */
export function CustomHeaderFormFields({
  form,
  kept,
}: {
  form: UseFormReturn<ServerFormValues>;
  kept: boolean;
}) {
  const [name, value] = form.watch(["customHeaderName", "customHeaderValue"]);
  return (
    <CustomHeaderFields
      value={{ name: name ?? "", value: value ?? "" }}
      kept={kept}
      error={form.formState.errors.customHeaderName?.message}
      onChange={(next) => {
        form.setValue("customHeaderName", next.name, { shouldDirty: true });
        form.setValue("customHeaderValue", next.value, { shouldDirty: true });
      }}
    />
  );
}
