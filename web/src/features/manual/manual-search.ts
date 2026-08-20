import type { ManualEntry } from "@/features/manual/api";

const sectionOrder = [
  "start",
  "authoring",
  "integrations",
  "governance",
  "cost",
  "operations",
  "security",
  "troubleshooting",
  "reference",
];

export function searchManual(pages: ManualEntry[], query: string) {
  const terms = words(query);
  if (terms.length === 0) return pages;
  return pages.filter((page) => terms.every((term) => searchable(page).includes(term)));
}

export function groupManualPages(pages: ManualEntry[]) {
  const groups = new Map<string, ManualEntry[]>();
  for (const page of pages) {
    const list = groups.get(page.section) ?? [];
    list.push(page);
    groups.set(page.section, list);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([section, entries]) => ({
      section,
      pages: entries.sort((a, b) => a.order - b.order),
    }));
}

export function manualSectionKeys(pages: ManualEntry[]) {
  return groupManualPages(pages).map((group) => group.section);
}

function searchable(page: ManualEntry) {
  return normalize(
    [
      page.title,
      page.summary,
      page.section,
      ...page.tags,
      ...page.headings.map((heading) => heading.title),
    ].join(" "),
  );
}

function words(query: string) {
  return normalize(query).split(/\s+/).filter(Boolean);
}

function normalize(value: string) {
  return value.normalize("NFD").replace(/\p{Diacritic}/gu, "").toLowerCase();
}

function rank(section: string) {
  const at = sectionOrder.indexOf(section);
  return at === -1 ? sectionOrder.length : at;
}
