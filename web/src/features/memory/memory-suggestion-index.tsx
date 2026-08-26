import type { MemorySuggestion } from "@/features/memory/api";
import { MemorySuggestionRow } from "@/features/memory/memory-suggestion-row";

export function MemorySuggestionIndex({
  suggestions,
  selectedID,
  onSelect,
}: {
  suggestions: MemorySuggestion[];
  selectedID?: string;
  onSelect: (suggestion: MemorySuggestion) => void;
}) {
  return (
    <div className="flex min-w-0 flex-col">
      {groups(suggestions, scopeOf).map((group) => (
        <section key={group.key}>
          <header className="sticky top-0 z-10 flex h-8 items-center justify-between border-b bg-muted px-3 text-2xs uppercase text-muted-foreground">
            <span className="min-w-0 truncate font-mono">{group.key}</span>
            <span className="tabular-nums">{group.items.length}</span>
          </header>
          {group.items.map((suggestion) => (
            <MemorySuggestionRow
              key={suggestion.id}
              suggestion={suggestion}
              selected={suggestion.id === selectedID}
              onSelect={() => onSelect(suggestion)}
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

function scopeOf(suggestion: MemorySuggestion): string {
  return `${suggestion.scope.company}/${suggestion.scope.area}`;
}
