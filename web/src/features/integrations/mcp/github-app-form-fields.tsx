import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import { GitHubAppFields } from "@/features/integrations/mcp/github-app-fields";
import type { GitHubAppInput } from "@/features/integrations/mcp/github-app-credential";

const FIELDS = [
  "githubAppId",
  "githubInstallationId",
  "githubPrivateKey",
  "githubRepositories",
  "githubAPIBaseURL",
  "githubToolsets",
] as const;

/**
 * The GitHub App fields inside the connection form. The schema owns the
 * values and their validation; this only adapts them to the shared fields.
 */
export function GitHubAppFormFields({ form }: { form: UseFormReturn<ServerFormValues> }) {
  const { t } = useTranslation();
  const watched = form.watch(FIELDS);
  const value = Object.fromEntries(
    FIELDS.map((name, i) => [name, watched[i] ?? ""]),
  ) as GitHubAppInput;
  const conflict = form.formState.errors.githubAppId?.message === "mcp.githubAppConflict";

  function onChange(next: GitHubAppInput) {
    for (const name of FIELDS) {
      if (next[name] !== value[name]) {
        form.setValue(name, next[name], { shouldDirty: true });
      }
    }
  }

  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p className="text-sm font-medium">{t("mcp.githubApp")}</p>
      <GitHubAppFields value={value} onChange={onChange} />
      {conflict && <p className="text-xs text-danger">{t("mcp.githubAppConflict")}</p>}
    </div>
  );
}
