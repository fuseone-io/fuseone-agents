import { useTranslation } from "react-i18next";
import { Wallet } from "lucide-react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { BarChart } from "@/components/shared/bar-chart";
import { Mono } from "@/components/shared/mono";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatMicros, formatTokens } from "@/lib/format";
import { CostKpis } from "@/features/cost/cost-kpis";
import { CostDrivers } from "@/features/cost/cost-drivers";
import { CostCaps } from "@/features/cost/cost-caps";
import { useCostRollup, useCostWindow } from "@/features/cost/api";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

export function CostPage() {
  const { t } = useTranslation();
  const window = useCostWindow(30);
  const daily = useCostRollup(window.from, window.to, "day");
  const byAgent = useCostRollup(window.from, window.to, "agent");
  const byArea = useCostRollup(window.from, window.to, "area");

  const error = daily.error ?? byAgent.error;
  const isLoading = daily.isLoading || byAgent.isLoading;
  const buckets = byAgent.data?.buckets ?? [];

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.cost}
        title="Custo e limites"
        description="Toda execução tem custo conhecido antes de alguém decidir escalá-la. A execução é a unidade contábil; agente, área e período são recortes dela."
      />

      <CostKpis daily={daily.data} isLoading={daily.isLoading} />

      {error ? (
        <ErrorState error={error} onRetry={() => void daily.refetch()} />
      ) : isLoading ? (
        <LoadingRows rows={4} />
      ) : buckets.length === 0 ? (
        <EmptyState
          icon={<Wallet className="size-6" />}
          title="Sem consumo no período"
          hint="O custo aparece assim que a primeira execução for concluída. Se as execuções já rodaram e o valor está zerado, o provedor de modelo está sem tabela de preços — a plataforma conta tokens e se recusa a chutar dinheiro."
        />
      ) : (
        <>
          <div className="flex flex-wrap gap-4">
            <Panel
              title="Gasto · últimos 14 dias"
              className="min-w-[320px] flex-[2_1_380px]"
            >
              <BarChart
                label="Gasto por dia nos últimos 14 dias"
                bars={(daily.data?.buckets ?? []).slice(-14).map((b) => ({
                  label: b.key,
                  value: b.cost.micros,
                  display: formatMicros(b.cost.micros),
                }))}
              />
            </Panel>

            <Panel
              title="De onde vem o custo"
              className="min-w-[260px] flex-[1_1_280px]"
            >
              <CostDrivers total={daily.data?.total} />
            </Panel>
          </div>

          <CostCaps byArea={byArea.data} />

          <Panel
            title="Por agente"
            action={
              <span className="text-xs text-muted-foreground">
                {t("cost.inPeriod")}
              </span>
            }
            flush
          >
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className={HEAD}>{t("cost.agent")}</TableHead>
                  <TableHead className={`${HEAD} text-right`}>
                    {t("cost.runs")}
                  </TableHead>
                  <TableHead className={`${HEAD} text-right`}>
                    {t("cost.average")}
                  </TableHead>
                  <TableHead className={`${HEAD} text-right`}>
                    {t("cost.tokens")}
                  </TableHead>
                  <TableHead className={`${HEAD} text-right`}>
                    {t("cost.cost")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {buckets.map((b) => (
                  <TableRow key={b.key} className="h-10 border-border-subtle">
                    <TableCell className="font-medium">{b.key}</TableCell>
                    <TableCell className={NUM}>{b.runs}</TableCell>
                    <TableCell className={NUM}>
                      {b.runs === 0
                        ? "—"
                        : formatMicros(Math.round(b.cost.micros / b.runs))}
                    </TableCell>
                    <TableCell className={NUM}>
                      <Mono dim>
                        {formatTokens(
                          (b.cost.inputTokens ?? 0) +
                            (b.cost.outputTokens ?? 0),
                        )}
                      </Mono>
                    </TableCell>
                    <TableCell className={NUM}>
                      {formatMicros(b.cost.micros)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>
        </>
      )}
    </>
  );
}
