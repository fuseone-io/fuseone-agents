import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { MCPServer } from "@/features/integrations/api";
import {
  githubAppIdsValid,
  readKeyFile,
  type GitHubAppInput,
} from "@/features/integrations/mcp/github-app-credential";

type StoredGitHubApp = NonNullable<MCPServer["githubApp"]>;

/**
 * The fields of a GitHub App credential, shared by the connection form and the
 * server's credential panel.
 *
 * The stored key is never shown, only that it is configured. Leaving the key
 * blank keeps it, which is why a stored App changes the key's hint rather
 * than pre-filling anything secret.
 */
export function GitHubAppFields({
  value,
  stored,
  onChange,
}: {
  value: GitHubAppInput;
  stored?: StoredGitHubApp | null;
  onChange: (next: GitHubAppInput) => void;
}) {
  const { t } = useTranslation();
  const set = (patch: Partial<GitHubAppInput>) => onChange({ ...value, ...patch });

  async function upload(files: FileList | null) {
    const file = files?.[0];
    if (file) set({ githubPrivateKey: await readKeyFile(file) });
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("mcp.githubAppHint")}</p>
      {stored && <StoredSummary stored={stored} />}
      <div className="grid gap-3 sm:grid-cols-2">
        <TextInput id="github-app-id" label={t("mcp.githubAppId")}
          value={value.githubAppId} onChange={(githubAppId) => set({ githubAppId })} />
        <TextInput id="github-installation-id" label={t("mcp.githubInstallationId")}
          value={value.githubInstallationId}
          onChange={(githubInstallationId) => set({ githubInstallationId })} />
      </div>
      {!githubAppIdsValid(value) && (
        <p className="text-xs text-danger">{t("mcp.githubAppIdsInvalid")}</p>
      )}
      <div className="space-y-1.5">
        <Label htmlFor="github-private-key">{t("mcp.githubPrivateKey")}</Label>
        <Textarea id="github-private-key" rows={4} autoComplete="off"
          className="font-mono text-xs"          value={value.githubPrivateKey}
          onChange={(e) => set({ githubPrivateKey: e.target.value })} />
        <Label htmlFor="github-private-key-file" className="text-xs">
          {t("mcp.githubPrivateKeyUpload")}
        </Label>
        <Input id="github-private-key-file" type="file" accept=".pem,.key,text/plain"
          onChange={(e) => void upload(e.target.files)} />
        <p className="text-xs text-muted-foreground">
          {stored ? t("mcp.githubPrivateKeyKept") : t("mcp.githubPrivateKeyHint")}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="github-repositories">{t("mcp.githubRepositories")}</Label>
        <Textarea id="github-repositories" rows={2} className="font-mono text-xs"
          value={value.githubRepositories}
          onChange={(e) => set({ githubRepositories: e.target.value })} />
        <p className="text-xs text-muted-foreground">{t("mcp.githubRepositoriesHint")}</p>
      </div>
      <div className="space-y-1.5">
        <TextInput id="github-api-base" label={t("mcp.githubAPIBaseURL")}
          value={value.githubAPIBaseURL}
          onChange={(githubAPIBaseURL) => set({ githubAPIBaseURL })} />
        <p className="text-xs text-muted-foreground">{t("mcp.githubAPIBaseURLHint")}</p>
      </div>
    </div>
  );
}

function StoredSummary({ stored }: { stored: StoredGitHubApp }) {
  const { t } = useTranslation();
  return (
    <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
      {t("mcp.githubAppStored", { app: stored.appId, installation: stored.installationId })}{" "}
      {t("mcp.githubPrivateKeyConfigured")}
    </p>
  );
}

function TextInput({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} autoComplete="off" className="font-mono"
        value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
