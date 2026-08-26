import type { DedupeRuling, Ruling } from "@/features/admin/classify-fields";
import type { Effect, Tool, ToolDedupe } from "@/features/admin/api";

export type ClassificationInput = {
  toolId: string;
  effect: Effect;
  untrusted: boolean;
  reason?: string;
  compensatedBy?: string;
  dedupe?: ToolDedupe;
  digest?: string;
};

export function blankRuling(): Ruling {
  return {
    effect: "",
    untrusted: true,
    reason: "",
    compensatedBy: "",
    dedupe: blankDedupe(),
  };
}

export function blankDedupe(): DedupeRuling {
  return { enabled: false, windowSeconds: "86400", argPaths: "" };
}

/*
startRulingFromTool is what a ruling opens on.

A tool nobody has judged opens blank: there is nothing to carry forward, and a
pre-filled effect would be the platform answering for the Curator.

A tool already ruled on opens on its ruling. The act is "change this", and
starting from the zero value makes the safest-looking answer — `read` — the one
a distracted person submits by touching nothing.
*/
export function startRulingFromTool(tool: Tool): Ruling {
  if (tool.effect === "unknown") return blankRuling();
  return {
    effect: tool.effect,
    untrusted: tool.untrusted,
    reason: "",
    compensatedBy: tool.compensatedBy ?? "",
    dedupe: tool.dedupe
      ? {
          enabled: true,
          windowSeconds: String(tool.dedupe.windowSeconds),
          argPaths: tool.dedupe.argPaths.join("\n"),
        }
      : blankDedupe(),
  };
}

export function classificationInput(
  tool: Tool,
  ruling: Ruling,
): ClassificationInput | null {
  const dedupe = dedupeFromRuling(ruling);
  if (ruling.effect === "" || dedupe === null) return null;
  const input: ClassificationInput = {
    toolId: tool.toolId,
    digest: tool.digest,
    effect: ruling.effect,
    untrusted: ruling.untrusted,
    reason: ruling.reason,
    compensatedBy: ruling.compensatedBy,
  };
  if (dedupe !== undefined) input.dedupe = dedupe;
  return input;
}

function dedupeFromRuling(ruling: Ruling): ToolDedupe | undefined | null {
  if (!ruling.dedupe.enabled) return undefined;
  const windowSeconds = Number(ruling.dedupe.windowSeconds);
  const argPaths = ruling.dedupe.argPaths
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter(Boolean);
  if (!Number.isInteger(windowSeconds) || windowSeconds < 1) return null;
  if (argPaths.length === 0) return null;
  return { windowSeconds, argPaths };
}
