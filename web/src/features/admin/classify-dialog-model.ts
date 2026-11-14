import type { DedupeRuling, Ruling } from "@/features/admin/classify-fields";
import type {
  Effect,
  RulingLevel,
  Tool,
  ToolDedupe,
  ToolRuling,
} from "@/features/admin/api";

export type ClassificationInput = {
  toolId: string;
  effect: Effect;
  untrusted: boolean;
  reason?: string;
  compensatedBy?: string;
  dedupe?: ToolDedupe;
  digest?: string;
  level?: RulingLevel;
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
startRulingFromTool is what a ruling opens on, for the level being recorded.

A tool nobody has judged, or whose ruling a new definition overtook, opens
blank: a pre-filled effect would be the platform answering for the Curator.

A tool already ruled on opens on the ruling of that level (NT-019). The
tool's own fields are the ruling in force, which under a ceiling is the
platform's: an owner started from them would save the platform's word over
their own and lose its compensation and dedupe. A level nobody ruled at opens
blank. A server that predates `rulings` answers for the platform's alone, so
its tool's own fields are that ruling.
*/
export function startRulingFromTool(
  tool: Tool,
  level: RulingLevel = "platform",
): Ruling {
  if (tool.effect === "unknown") return blankRuling();
  if (tool.rulings === undefined) {
    return level === "platform"
      ? rulingOf({ ...tool, effect: tool.effect })
      : blankRuling();
  }
  const ruled = tool.rulings.find((one) => one.level === level);
  return ruled ? rulingOf(ruled) : blankRuling();
}

function rulingOf(said: Pick<
  ToolRuling,
  "effect" | "untrusted" | "compensatedBy" | "dedupe"
>): Ruling {
  return {
    effect: said.effect,
    untrusted: said.untrusted,
    reason: "",
    compensatedBy: said.compensatedBy ?? "",
    dedupe: said.dedupe
      ? {
          enabled: true,
          windowSeconds: String(said.dedupe.windowSeconds),
          argPaths: said.dedupe.argPaths.join("\n"),
        }
      : blankDedupe(),
  };
}

/*
The level travels only when it is the owner's. Absent is the platform's on the
server, so a platform curator's ruling is the request it always was.
*/
export function classificationInput(
  tool: Tool,
  ruling: Ruling,
  level: RulingLevel = "platform",
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
  if (level === "owner") input.level = "owner";
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

/*
A suggestion accepted fills the form and keeps the reason the curator's.

No fallback on the effect. The contract makes it required, so the only way
`??` fires is a suggestion that arrived without one — and it would fire into
`read`, the value this dialog stopped defaulting to everywhere else. An empty
effect leaves the form unanswered, the honest reading of a suggestion that
suggests nothing. Accepting a suggestion is still a decision somebody signs,
and signing somebody else's sentence is not the same as writing one.
*/
export function rulingFromSuggestion(tool: Tool, current: Ruling): Ruling {
  return {
    effect: tool.suggested?.effect ?? "",
    untrusted: tool.suggested?.untrusted ?? true,
    compensatedBy: tool.suggested?.compensatedBy ?? "",
    dedupe: current.dedupe,
    reason: "",
  };
}
