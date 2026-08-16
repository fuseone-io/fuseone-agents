import { useTranslation } from "react-i18next";
import { useState } from "react";
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
export function Waiting({ tools }: { tools: Tool[] }) {
  const { t } = useTranslation();
  // Both refusals count. A ruling overtaken by a new definition blocks the
  // tool exactly as never having ruled does, and a count that left it out
  // would say the queue was empty while agents were being stopped.
  const waiting = tools.filter(
    (tool) => tool.effect === "unknown" || tool.stale,
  ).length;
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

export function ToolsPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useTools();
  const [classifying, setClassifying] = useState<Tool | null>(null);
  const tools = data?.items ?? [];

  return (
    <Panel
      title={t("admin.tools")}
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
            title={t("admin.noToolsFound")}
            hint={t("admin.toolsEmptyHint")}
          />
        </div>
      ) : (
        <Table>
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
            {tools.map((tool) => (
              <TableRow key={tool.toolId} className="h-10 border-border-subtle">
                <TableCell>
                  <Mono className={tool.offered === false ? "opacity-60" : ""}>
                    {tool.toolId}
                  </Mono>
                  {tool.description && (
                    <div className="truncate text-xs text-muted-foreground">
                      {tool.description}
                    </div>
                  )}
                </TableCell>
                {/* The list is what this installation has ever offered and
                    never shrinks — two workers connected to different servers
                    would delete each other's rows if it did. Whether a tool
                    can be called now is a fact about its server, said here
                    rather than left for somebody to infer from silence. */}
                <TableCell className="text-muted-foreground">
                  <span className="flex items-center gap-1.5">
                    {tool.server}
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
      )}

      <ClassifyDialog
        tool={classifying}
        tools={tools}
        onClose={() => setClassifying(null)}
      />
    </Panel>
  );
}
