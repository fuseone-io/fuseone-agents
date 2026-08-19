import { CompanyFooter } from "@/features/companies/company-footer";
import { CompanyRow } from "@/features/companies/company-row";
import { CompanyTableHeader } from "@/features/companies/company-table-header";
import type { Company } from "@/features/companies/api";

export function CompanyList({
  companies,
  openRows,
  shown,
  total,
  hasMore,
  onOpenChange,
  onEdit,
  onToggleArchived,
  onLoadMore,
}: {
  companies: Company[];
  openRows: Record<string, boolean>;
  shown: number;
  total: number;
  hasMore: boolean;
  onOpenChange: (company: string, open: boolean) => void;
  onEdit: (company: string) => void;
  onToggleArchived: (company: Company) => void;
  onLoadMore: () => void;
}) {
  return (
    <>
      <ul className="divide-y divide-border-subtle">
        <CompanyTableHeader />
        {companies.map((company) => (
          <li key={company.id}>
            <CompanyRow
              company={company}
              open={Boolean(openRows[company.id])}
              onOpenChange={(open) => onOpenChange(company.id, open)}
              onEdit={() => onEdit(company.id)}
              onToggleArchived={() => onToggleArchived(company)}
            />
          </li>
        ))}
      </ul>
      <CompanyFooter
        shown={shown}
        total={total}
        hasMore={hasMore}
        onLoadMore={onLoadMore}
      />
    </>
  );
}
