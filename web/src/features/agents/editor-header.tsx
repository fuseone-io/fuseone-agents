import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Ellipsis, FlaskConical, History } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useSidebar } from "@/components/ui/sidebar";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import {
  PageActions,
  PageIdentity,
} from "@/components/layout/page-actions";

/**
 * Which agent is being written, in the header that is already there.
 *
 * Rendered into the shell's own 52px row rather than under it: two headers
 * stacked is what the screen had, and the second one was repeating what the
 * breadcrumb had just said.
 *
 * Inside an editor the global navigation is noise, so the sidebar collapses
 * to its icon rail — and is put back exactly as it was found, because a
 * screen that quietly changes somebody's layout preference is a screen they
 * stop trusting with the rest.
 */
export function EditorHeader({
  agentId,
  name,
  version,
  stage,
}: {
  agentId: string;
  name: string;
  version?: string;
  stage: string;
}) {
  const { t } = useTranslation();
  const { open, setOpen } = useSidebar();

  useEffect(() => {
    const was = open;
    setOpen(false);
    return () => setOpen(was);
    // Once, on entering the editor. Following `open` would fight somebody who
    // opened the rail on purpose while working here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <>
      <PageIdentity>
        <span className="truncate text-sm font-medium">{name}</span>
        {version && (
          <span className="rounded-md border border-border bg-muted px-1.5 py-px">
            <Mono dim className="text-[11px]">
              {version.slice(0, 9)}
            </Mono>
          </span>
        )}
        <span className="flex h-[22px] items-center gap-1.5 rounded-pill border border-border bg-muted px-2.5 text-2xs text-text-secondary">
          <StateDot state="draft" />
          {stage}
        </span>
      </PageIdentity>

      <PageActions>
        {agentId !== "" && (
          <>
            <Button variant="ghost" size="sm" asChild className="h-8">
              <Link to={`/agents/${agentId}`}>
                <History className="size-4" aria-hidden />
                {t("agents.versionsItem")}
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild className="h-8">
              <Link to={`/agents/${agentId}/simulate`}>
                <FlaskConical className="size-4" aria-hidden />
                {t("agents.simulate")}
              </Link>
            </Button>
            {/* Everything that is not one of those two, so the row stays
                three items wide however much the screen learns to do. */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label={t("agents.moreActions")}
                >
                  <Ellipsis className="size-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link to={`/agents/${agentId}`}>{t("agents.openAgent")}</Link>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        )}
      </PageActions>
    </>
  );
}
