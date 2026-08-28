import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router-dom";
import { Pencil } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { RemoveButton } from "@/components/shared/remove-button";
import { effectOf, stateOf } from "@/features/policies/policy-effect";
import { cn } from "@/lib/utils";
import type { Policy } from "@/lib/api/client";
import { draftSentence } from "@/features/policies/policy-sentence";

/**
 * The rules in force, as a table an owner reads down.
 *
 * The sentence is the widest column because it is the rule: everything else on
 * the row — the code, the owner, the count — is how you find it again.
 */
export function PoliciesTable({
  policies,
  canManage,
  deletingCode,
  onDelete,
}: {
  policies: Policy[];
  canManage: boolean;
  deletingCode?: string;
  onDelete: (code: string) => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <Table>
      <TableHeader>
        <TableRow className="border-border-subtle hover:bg-transparent">
          <TableHead className="text-2xs uppercase tracking-label">
            {t("policies.code")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("policies.rule")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("policies.effect")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("policies.state")}
          </TableHead>
          <TableHead className="text-2xs uppercase tracking-label">
            {t("policies.owner")}
          </TableHead>
          <TableHead className="text-right text-2xs uppercase tracking-label">
            {t("policies.decisions")}
          </TableHead>
          {canManage && (
            <TableHead className="w-24 text-right text-2xs uppercase tracking-label">
              {t("common.actions")}
            </TableHead>
          )}
        </TableRow>
      </TableHeader>

      <TableBody>
        {policies.map((policy) => (
          <TableRow
            key={policy.code}
            className="h-11 cursor-pointer border-border-subtle"
            onClick={(event) => {
              if ((event.target as HTMLElement).closest("a, button")) return;
              void navigate(`/policies/${policy.code}`);
            }}
          >
            <TableCell>
              <Link to={`/policies/${policy.code}`} className="hover:underline">
                <Mono>{policy.code}</Mono>
              </Link>
            </TableCell>

            <TableCell className="max-w-[420px]">
              <Link
                to={`/policies/${policy.code}`}
                className="block hover:underline"
              >
                <div className="truncate text-sm font-medium">
                  {policy.name}
                </div>
                {/* Generated from the fields the Gate reads, so this row
                    cannot describe a rule the engine does not run. */}
                <Mono dim className="block truncate text-2xs">
                  {draftSentence(policy, t)}
                </Mono>
              </Link>
            </TableCell>

            <TableCell>
              <span
                className={cn(
                  "rounded-pill px-2 py-0.5 text-2xs font-medium",
                  effectOf(policy).className,
                )}
              >
                {t(effectOf(policy).label)}
              </span>
            </TableCell>

            <TableCell>
              <span className={cn("text-xs", stateOf(policy).className)}>
                {t(stateOf(policy).label)}
              </span>
            </TableCell>

            <TableCell className="text-xs text-muted-foreground">
              {policy.owner || "—"}
            </TableCell>

            <TableCell className="text-right">
              <Mono dim>{policy.hits ?? 0}</Mono>
            </TableCell>

            {canManage && (
              <TableCell className="text-right">
                <div
                  className="flex justify-end gap-1"
                  onClick={(event) => event.stopPropagation()}
                >
                  <Button variant="ghost" size="icon" asChild>
                    <Link
                      to={`/policies/${policy.code}`}
                      aria-label={t("policies.editNamed", { name: policy.name })}
                    >
                      <Pencil className="size-4" aria-hidden />
                    </Link>
                  </Button>
                  <RemoveButton
                    title={t("policies.removeNamed", { name: policy.name })}
                    description={t("policies.removeDescription", {
                      code: policy.code,
                    })}
                    disabled={deletingCode === policy.code}
                    onConfirm={() => onDelete(policy.code)}
                  />
                </div>
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
