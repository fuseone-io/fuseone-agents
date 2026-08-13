import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
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
export function PoliciesTable({ policies }: { policies: Policy[] }) {
  const { t } = useTranslation();
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
        </TableRow>
      </TableHeader>

      <TableBody>
        {policies.map((policy) => (
          <TableRow key={policy.code} className="h-11 border-border-subtle">
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
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
