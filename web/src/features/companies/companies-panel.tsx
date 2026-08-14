import { Building2, RotateCcw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Mono } from "@/components/shared/mono";
import { Panel } from "@/components/shared/panel";
import { CompanyForm } from "@/features/companies/company-form";
import { useCompanies, useUpdateCompany } from "@/features/companies/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The companies this installation governs.
 *
 * The screen only exists for somebody who holds authority over the
 * installation, and the API says so rather than the console guessing: a
 * refusal here is a real answer and is shown as one, because hiding the
 * section would leave an operator wondering where it went.
 *
 * Withdrawn companies stay in the list. Their runs are still readable and
 * somebody looking at those has to be able to find out what that company was.
 */
export function CompaniesPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useCompanies();
  const [adding, setAdding] = useState(false);
  const update = useUpdateCompany();

  const companies = data?.items ?? [];

  return (
    <Panel
      title={t("companies.companies")}
      action={
        <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
          {t("companies.register")}
        </Button>
      }
      flush
    >
      {isLoading ? (
        <div className="p-4">
          <LoadingRows rows={3} />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : companies.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Building2 className="size-6" />}
            title={t("companies.none")}
            hint={t("companies.noneHint")}
          />
        </div>
      ) : (
        <ul className="flex flex-col">
          {companies.map((company) => (
            <li
              key={company.id}
              className="flex items-center gap-3 border-b border-border-subtle px-4 py-2.5 last:border-0"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{company.label}</p>
                <Mono dim className="block truncate text-2xs">
                  {company.id}
                </Mono>
              </div>

              <span className="shrink-0 text-2xs tabular-nums text-muted-foreground">
                {t("companies.areaCount", { count: company.areas })}
              </span>

              {company.archived ? (
                <>
                  <Badge variant="outline">{t("companies.withdrawn")}</Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      update.mutate({ company: company.id, archived: false })
                    }
                  >
                    <RotateCcw className="size-3.5" aria-hidden />
                    {t("companies.restore")}
                  </Button>
                </>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    update.mutate(
                      { company: company.id, archived: true },
                      { onError: (e) => toast.error(problemMessage(e, t)) },
                    )
                  }
                >
                  {t("companies.withdraw")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      {adding && <CompanyForm onClose={() => setAdding(false)} />}
    </Panel>
  );
}
