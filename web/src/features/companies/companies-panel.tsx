import { Building2 } from "lucide-react";
import { useDeferredValue, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import {
  matchesCompany,
  matchesCompanyView,
  type CompanyView,
} from "@/features/companies/company-filters";
import { CompanyForm } from "@/features/companies/company-form";
import { CompanyHeader } from "@/features/companies/company-header";
import { CompanyList } from "@/features/companies/company-list";
import { CompanyToolbar } from "@/features/companies/company-toolbar";
import {
  useCompanies,
  useUpdateCompany,
  type Company,
} from "@/features/companies/api";
import { useVisibleItems } from "@/hooks/use-visible-items";
import { problemMessage } from "@/lib/api/problem-message";

const EMPTY_COMPANIES: Company[] = [];

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
  const [editing, setEditing] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [view, setView] = useState<CompanyView>("all");
  const [openRows, setOpenRows] = useState<Record<string, boolean>>({});
  const update = useUpdateCompany();

  const companies = data?.items ?? EMPTY_COMPANIES;
  const query = useDeferredValue(search.trim().toLowerCase());
  const filtered = useMemo(
    () =>
      companies.filter(
        (company) =>
          matchesCompanyView(company, view) &&
          (!query || matchesCompany(company, query, t)),
      ),
    [companies, query, t, view],
  );
  const page = useVisibleItems(filtered, 50);
  const active = companies.filter((company) => !company.archived).length;
  const withdrawn = companies.length - active;
  const noMatches = companies.length > 0 && filtered.length === 0;
  const companyEditing = companies.find((company) => company.id === editing);

  return (
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <CompanyHeader onAdd={() => setAdding(true)} />

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
      ) : noMatches ? (
        <>
          <CompanyToolbar
            search={search}
            onSearch={setSearch}
            view={view}
            onView={setView}
            active={active}
            withdrawn={withdrawn}
          />
          <div className="p-8">
            <EmptyState
              icon={<Building2 className="size-6" />}
              title={t("companies.noMatches")}
              hint={t("companies.noMatchesHint")}
            />
          </div>
        </>
      ) : (
        <>
          <CompanyToolbar
            search={search}
            onSearch={setSearch}
            view={view}
            onView={setView}
            active={active}
            withdrawn={withdrawn}
          />
          <CompanyList
            companies={page.visible}
            openRows={openRows}
            shown={page.loaded}
            total={companies.length}
            hasMore={page.hasMore}
            onOpenChange={(company, open) =>
              setOpenRows((rows) => ({ ...rows, [company]: open }))
            }
            onEdit={setEditing}
            onToggleArchived={(company) =>
              update.mutate(
                { company: company.id, archived: !company.archived },
                {
                  onSuccess: () =>
                    toast.success(
                      company.archived
                        ? t("companies.restored")
                        : t("companies.withdrawnNow"),
                    ),
                  onError: (error) => toast.error(problemMessage(error, t)),
                },
              )
            }
            onLoadMore={page.loadMore}
          />
        </>
      )}

      {adding && <CompanyForm onClose={() => setAdding(false)} />}
      {companyEditing && (
        <CompanyForm company={companyEditing} onClose={() => setEditing(null)} />
      )}
    </section>
  );
}
