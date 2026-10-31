import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import type { MCPGitHubApp, MCPServer } from "@/features/integrations/api";
import { GitHubAppFields } from "@/features/integrations/mcp/github-app-fields";
import {
  blankGitHubApp,
  githubAppComplete,
  githubAppFromValue,
  githubAppHeaders,
  type WriteGitHubApp,
} from "@/features/integrations/mcp/github-app-credential";

/**
 * The GitHub App credential of an existing server, with its own save and its
 * own removal.
 *
 * Apart from the token and OAuth fields because it is a different act: an App
 * replaces either of them when saved, and saving it from the same button as a
 * token would make "which one did I just store" a question.
 */
export function GitHubAppSection({
  server,
  pending,
  write,
}: {
  server: MCPServer;
  pending: boolean;
  write: WriteGitHubApp;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(blankGitHubApp);
  const stored = server.githubApp ?? null;

  async function save(githubApp: MCPGitHubApp, headers?: Record<string, string>) {
    if (await write({ githubApp, headers })) setValue(blankGitHubApp());
  }

  return (
    <div className="space-y-3">
      <Separator />
      <h3 className="text-sm font-medium">{t("mcp.githubApp")}</h3>
      <GitHubAppFields value={value} stored={stored} onChange={setValue} />
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={pending || !githubAppComplete(value, stored?.hasPrivateKey === true)}
          onClick={() => void save(githubAppFromValue(value), githubAppHeaders(value))}
        >
          {t("mcp.saveGitHubApp")}
        </Button>
        {stored && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="text-danger"
            disabled={pending}
            onClick={() => void save({}, {})}
          >
            <Trash2 className="size-3.5" />
            {t("mcp.revokeGitHubApp")}
          </Button>
        )}
      </div>
    </div>
  );
}
