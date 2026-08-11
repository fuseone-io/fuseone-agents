import { formatTokens } from "@/lib/format";
import type { components } from "@/lib/api/schema.gen";

type Cost = components["schemas"]["Cost"];

/**
 * What the money went on.
 *
 * The breakdown is the reason the contract separates the token kinds: a cache
 * read costs a fraction of an input token, so a total alone bills an agent
 * without explaining it. This is the screen where that split earns its keep.
 */
export function CostDrivers({ total }: { total?: Cost }) {
  if (!total) return null;

  const drivers = [
    { label: "Entrada", value: total.inputTokens ?? 0 },
    { label: "Saída", value: total.outputTokens ?? 0 },
    { label: "Leitura de cache", value: total.cacheReadTokens ?? 0 },
    { label: "Escrita de cache", value: total.cacheWriteTokens ?? 0 },
  ].filter((d) => d.value > 0);

  const sum = drivers.reduce((acc, d) => acc + d.value, 0);
  if (sum === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Nenhum token contabilizado no período.
      </p>
    );
  }

  return (
    <ul className="flex flex-col gap-3">
      {drivers.map((driver) => {
        const share = Math.round((driver.value / sum) * 100);
        return (
          <li key={driver.label}>
            <div className="mb-1 flex justify-between gap-2 text-xs">
              <span>{driver.label}</span>
              <span className="whitespace-nowrap font-mono tabular-nums text-muted-foreground">
                {formatTokens(driver.value)} · {share}%
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-pill bg-muted">
              <div className="h-full rounded-pill bg-primary" style={{ width: `${share}%` }} />
            </div>
          </li>
        );
      })}
    </ul>
  );
}
