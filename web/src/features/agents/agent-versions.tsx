import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Mono } from "@/components/shared/mono";
import { formatInstant } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { AgentVersion } from "@/lib/api/client";

/**
 * Every version ever published, newest first.
 *
 * Nothing here deletes or edits: publishing adds, and the older versions stay
 * because they are the only correct explanation of the runs pinned to them.
 */
export function AgentVersions({
  agentId,
  versions,
  current,
}: {
  agentId: string;
  versions: AgentVersion[];
  current: string;
}) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        Versões ({versions.length})
      </h2>

      <ol className="flex flex-col">
        {versions.map((version) => (
          <li key={version.versionId}>
            <Link
              to={`/agents/${agentId}?version=${version.versionId}`}
              className={cn(
                "flex items-baseline gap-2 rounded-md px-1 py-1.5 hover:bg-muted",
                version.versionId === current && "bg-muted",
              )}
            >
              <Mono>{version.versionId.slice(0, 9)}</Mono>
              {version.latest && (
                <span className="rounded-pill bg-success-surface px-1.5 text-2xs text-success">
                  {t("agents.currentVersion")}
                </span>
              )}
              <span className="ml-auto shrink-0 text-2xs text-muted-foreground">
                {formatInstant(version.publishedAt)}
              </span>
            </Link>
          </li>
        ))}
      </ol>
    </section>
  );
}
