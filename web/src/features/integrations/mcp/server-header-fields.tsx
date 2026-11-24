import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { FormLabel } from "@/components/ui/form";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/** The headers a recipe names, each its own secret, inside the connect form. */
export function ServerHeaderFields({
  form,
  headers,
  kept,
}: {
  form: UseFormReturn<ServerFormValues>;
  headers: string[];
  kept: boolean;
}) {
  const { t } = useTranslation();
  const error = form.formState.errors.headers;
  const message = typeof error?.message === "string" ? error.message : undefined;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="grid gap-3 sm:grid-cols-2">
        {headers.map((header) => (
          <div key={header} className="min-w-0 space-y-1.5">
            <FormLabel htmlFor={headerInputID(header)}>{header}</FormLabel>
            <Input
              id={headerInputID(header)}
              type="password"
              autoComplete="off"
              className="font-mono"
              value={(form.watch("headers") ?? {})[header] ?? ""}
              onChange={(event) =>
                form.setValue(
                  "headers",
                  { ...(form.getValues("headers") ?? {}), [header]: event.target.value },
                  { shouldDirty: true, shouldValidate: true },
                )
              }
            />
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {kept ? t("integrations.tokenKept") : t("mcp.remoteHeadersHint")}
      </p>
      {message && <p className="text-xs text-danger">{t(message)}</p>}
    </div>
  );
}

function headerInputID(header: string) {
  return `header-${header.replace(/[^A-Za-z0-9_-]/g, "-")}`;
}
