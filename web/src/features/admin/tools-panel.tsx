import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { useMemo, useState } from "react";
import { Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Panel } from "@/components/shared/panel";
import { LoadMore } from "@/components/shared/load-more";
import { Mono } from "@/components/shared/mono";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { EffectBadge } from "@/features/admin/effect-badge";
import { ClassifyDialog } from "@/features/admin/classify-dialog";
import { useTools, type Tool } from "@/features/admin/api";
import { Badge } from "@/components/ui/badge";
import { useVisibleItems } from "@/hooks/use-visible-items";
import { waitingFor } from "@/features/admin/waiting-tools";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";

/**
 * How many tools are waiting for a ruling, and what that means for them.
 *
 * The panel used to say tools "arrive as reads", which stopped being true when
 * they started arriving unclassified and refused. A screen still describing
 * the behaviour it had before is worse than one saying nothing: somebody reads
 * it, believes the tool is usable, and goes looking for the fault somewhere
 * else.
 *
 * A count rather than a note, because the number is the work. Zero says the
 * queue is empty, which is also worth being able to see.
 */
/*
What is waiting, named once.

Both refusals count. A tool nobody ruled on and a tool whose ruling was
overtaken are stopped by the Gate alike, and a count that left the second out
would say the queue was empty while agents were being stopped.
*/
export function Waiting({ tools }: { tools: Tool[] }) {
  const { t } = useTranslation();
  const waiting = waitingFor(tools).length;
  return (
    <span
      className={
        waiting > 0 ? "text-xs text-danger" : "text-xs text-muted-foreground"
      }
    >
      {waiting > 0
        ? t("admin.waitingForARuling", { count: waiting })
        : t("admin.arriveUnclassified")}
    </span>
  );
}

/*
The Curator's queue, and not a second catalogue.

Every tool with its ruling now lives on the server that offers it, which is
where the surrounding facts are: what else that server brought in, who declares
it, what the recipe suggested. Listing them all again here was the same rows in
two places.

What this answers instead is the question no per-server page can: across the
whole installation, what is waiting. Ten servers is ten visits to discover
there is nothing to do, and a queue that is empty says so at a glance.

Waiting means refused. A tool nobody ruled on and a tool whose ruling was
overtaken are both stopped by the Gate, and they are different work — one is a
decision to make, the other a decision to check.
*/
export function ToolsPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useTools();
  const [classifying, setClassifying] = useState<Tool | null>(null);
  const tools = useMemo(() => waitingFor(data?.items ?? []), [data]);
  const page = useVisibleItems(tools, 50);

  return (
    <Panel
      title={t("admin.toolsWaiting")}
      action={<Waiting tools={tools} />}
      flush
    >
      {isLoading ? (
        <div className="p-4">
          <LoadingRows />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : tools.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Wrench className="size-6" />}
            title={t("admin.nothingWaiting")}
            hint={t("admin.nothingWaitingHint")}
          />
        </div>
      ) : (
        <>
          <Table className="min-w-[1040px] table-fixed">
            <colgroup>
              <col className="w-[400px]" />
              <col className="w-[160px]" />
              <col className="w-[112px]" />
              <col className="w-[96px]" />
              <col className="w-[152px]" />
              <col className="w-[120px]" />
            </colgroup>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className={HEAD}>{t("admin.tool")}</TableHead>
                <TableHead className={HEAD}>{t("admin.server")}</TableHead>
                <TableHead className={HEAD}>{t("admin.effect")}</TableHead>
                <TableHead className={HEAD}>{t("admin.untrusted")}</TableHead>
                <TableHead className={HEAD}>{t("admin.undoColumn")}</TableHead>
                <TableHead className={`${HEAD} text-right`} />
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.visible.map((tool) => (
                <TableRow
                  key={tool.toolId}
                  className="h-10 border-border-subtle"
                >
                  <TableCell className="max-w-0">
                    <Mono
                      className={
                        tool.offered === false
                          ? "block truncate opacity-60"
                          : "block truncate"
                      }
                    >
                      {tool.toolId}
                    </Mono>
                    {tool.description && (
                      <div
                        className="truncate text-xs text-muted-foreground"
                        title={tool.description}
                      >
                        {tool.description}
                      </div>
                    )}
                  </TableCell>
                  {/* The list is what this installation has ever offered and
                    never shrinks — two workers connected to different servers
                    would delete each other's rows if it did. Whether a tool
                    can be called now is a fact about its server, said here
                    rather than left for somebody to infer from silence. */}
                  {/* Where the decision belongs. The queue says what is
                    waiting; the server's own page is where the rest of the
                    context is. */}
                  <TableCell className="max-w-0 text-muted-foreground">
                    <span className="flex min-w-0 items-center gap-1.5">
                      <Link
                        to={`/integrations/mcp/${tool.server}`}
                        className="min-w-0 truncate underline underline-offset-2"
                      >
                        {tool.server}
                      </Link>
                      {tool.offered === false && (
                        <Badge
                          variant="outline"
                          className="rounded-pill border-transparent bg-warning-surface text-2xs font-normal text-warning"
                        >
                          {t("admin.notOffered")}
                        </Badge>
                      )}
                    </span>
                  </TableCell>
                  <TableCell>
                    <EffectBadge effect={tool.effect} stale={tool.stale} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {tool.untrusted ? t("common.yes") : t("common.no")}
                  </TableCell>
                  {/* A ruling nobody can see from the outside is a ruling that
                    gets made twice. An em dash rather than a blank: nothing
                    undoes this tool is an answer, not a missing field. */}
                  <TableCell>
                    {tool.compensatedBy ? (
                      <Mono className="text-xs">{tool.compensatedBy}</Mono>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setClassifying(tool)}
                    >
                      {t("admin.classify")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <div className="px-4 pb-3">
            <LoadMore
              loaded={page.loaded}
              total={page.total}
              hasMore={page.hasMore}
              isLoading={false}
              onLoad={page.loadMore}
            />
          </div>
        </>
      )}

      <ClassifyDialog
        tool={classifying}
        tools={tools}
        onClose={() => setClassifying(null)}
      />
    </Panel>
  );
}
