import type { MemoryAssertion } from "@/features/memory/api";
import { MemoryAssertionRow } from "@/features/memory/memory-assertion-row";

export function MemoryAssertionIndex({
  assertions,
  selectedID,
  onSelect,
}: {
  assertions: MemoryAssertion[];
  selectedID?: string;
  onSelect: (assertion: MemoryAssertion) => void;
}) {
  return (
    <div className="flex min-w-0 flex-col">
      {groups(assertions, scopeOf).map((group) => (
        <section key={group.key}>
          <header className="sticky top-0 z-10 flex h-8 items-center justify-between border-b bg-muted px-3 text-2xs uppercase text-muted-foreground">
            <span className="min-w-0 truncate font-mono">{group.key}</span>
            <span className="tabular-nums">{group.items.length}</span>
          </header>
          {group.items.map((assertion) => (
            <MemoryAssertionRow
              key={assertion.id}
              assertion={assertion}
              selected={assertion.id === selectedID}
              onSelect={() => onSelect(assertion)}
            />
          ))}
        </section>
      ))}
    </div>
  );
}

function groups<T>(items: T[], keyOf: (item: T) => string) {
  const byKey = new Map<string, T[]>();
  for (const item of items) {
    const key = keyOf(item);
    const group = byKey.get(key);
    if (group) {
      group.push(item);
      continue;
    }
    byKey.set(key, [item]);
  }
  return Array.from(byKey, ([key, groupItems]) => ({ key, items: groupItems }));
}

function scopeOf(assertion: MemoryAssertion): string {
  return `${assertion.scope.company}/${assertion.scope.area}`;
}
