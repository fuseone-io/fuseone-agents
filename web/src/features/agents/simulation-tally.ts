import type {
  SimulationCase,
  SimulationReport,
} from "@/features/agents/simulation-api";

/**
 * The line an author reads before any of the rows.
 *
 * Derived here rather than sent, because every number in it is already in the
 * rows: a count the server computed and the table disagreed with would be one
 * more thing to keep in step, and the reader would have no way to tell which
 * was right.
 */
export interface Tally {
  cases: number;
  finished: number;
  parked: number;
  waiting: number;
  running: number;
  /** Cases the Gate refused at least once, wherever they ended. */
  stopped: number;
  micros: number;
}

export function tally(report: SimulationReport): Tally {
  const counted: Tally = {
    cases: report.cases.length,
    finished: 0,
    parked: 0,
    waiting: 0,
    running: 0,
    stopped: 0,
    micros: 0,
  };

  for (const c of report.cases) {
    counted.micros += c.cost.micros;
    if (stoppedByGate(c)) counted.stopped++;

    switch (c.settled) {
      case "finished":
        counted.finished++;
        break;
      case "parked":
        counted.parked++;
        break;
      case "awaiting_approval":
        counted.waiting++;
        break;
      default:
        counted.running++;
    }
  }
  return counted;
}

/**
 * A case that did not go cleanly through to a held answer.
 *
 * Gate refusals are counted even if the run later finished: that is the
 * interesting rehearsal finding, and hiding it under "finished" is how a
 * blocked action reaches publishing review as a green row.
 */
export function caseNeedsLook(c: SimulationCase): boolean {
  return (
    c.settled !== "finished" ||
    (c.unmet?.length ?? 0) > 0 ||
    c.error !== undefined ||
    c.reason !== undefined ||
    stoppedByGate(c)
  );
}

/** A case the Gate refused at least once — counted apart from where it ended,
 *  because a run that was refused and carried on still needs looking at. */
export function stoppedByGate(c: SimulationCase): boolean {
  return (c.acted ?? []).some((act) => act.verdict === "block");
}

/**
 * How many cases a file holds, counted the way the server counts them.
 *
 * Blank lines are not cases: every export ends in a newline, and counting it
 * would promise the author one more case than they are going to get.
 */
export function countCases(file: string): number {
  return file.split("\n").filter((line) => line.trim() !== "").length;
}
