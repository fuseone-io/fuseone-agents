import { describe, expect, it } from "vitest";
import type { ManualEntry } from "@/features/manual/api";
import { groupManualPages, searchManual } from "@/features/manual/manual-search";

const pages: ManualEntry[] = [
  page("agents", "Agents", "start", ["version"], ["A run is pinned"]),
  page("slack", "Slack", "integrations", ["xapp", "socket"], ["Socket Mode"]),
  page("cost", "Costs", "cost", ["budget"], ["Market defaults"]),
];
const [agents, slack, cost] = pages as [ManualEntry, ManualEntry, ManualEntry];

describe("manual search", () => {
  it("searches metadata the index carries without loading page bodies", () => {
    expect(searchManual(pages, "xapp").map((entry) => entry.slug)).toEqual(["slack"]);
    expect(searchManual(pages, "market").map((entry) => entry.slug)).toEqual(["cost"]);
  });

  it("keeps sections in the order readers expect", () => {
    expect(groupManualPages([cost, slack, agents]).map((group) => group.section)).toEqual([
      "start",
      "integrations",
      "cost",
    ]);
  });
});

function page(
  slug: string,
  title: string,
  section: string,
  tags: string[],
  headings: string[],
): ManualEntry {
  return {
    slug,
    title,
    section,
    tags,
    summary: `${title} summary`,
    order: 1,
    headings: headings.map((heading) => ({ id: heading.toLowerCase(), title: heading, level: 2 })),
  };
}
