import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { GrantInput } from "@/features/admin/people-api";

const ROLES: GrantInput["role"][] = [
  "admin",
  "author",
  "approver",
  "curator",
  "auditor",
];

/** One grant somebody was given directly: where it applies, and as what. */
export function GrantRow({
  grant,
  index,
  onChange,
  onRemove,
}: {
  grant: GrantInput;
  index: number;
  onChange: (patch: Partial<GrantInput>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  // Every field keeps its label; only the first row shows one. Four rows of
  // repeated headings is noise, and dropping them outright would leave the
  // inputs unnamed to anyone not reading the column by eye.
  const heading = index === 0 ? "text-xs" : "sr-only";

  return (
    <div className="grid grid-cols-[1fr_1fr_140px_32px] items-end gap-2">
      <Cell
        id={`grant-company-${index}`}
        label={t("identity.company")}
        labelClass={heading}
        value={grant.company}
        onChange={(company) => onChange({ company })}
      />
      <Cell
        id={`grant-area-${index}`}
        label={t("identity.area")}
        labelClass={heading}
        value={grant.area}
        onChange={(area) => onChange({ area })}
      />

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`grant-role-${index}`} className={heading}>
          {t("identity.role")}
        </Label>
        <Select
          value={grant.role}
          onValueChange={(role) =>
            onChange({ role: role as GrantInput["role"] })
          }
        >
          <SelectTrigger id={`grant-role-${index}`} className="!h-9 w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ROLES.map((role) => (
              <SelectItem key={role} value={role}>
                {t(`roles.${role}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Button
        variant="ghost"
        size="icon"
        className="size-9 text-muted-foreground"
        onClick={onRemove}
      >
        <X className="size-4" aria-hidden />
        <span className="sr-only">{t("people.removeGrant")}</span>
      </Button>
    </div>
  );
}

function Cell({
  id,
  label,
  labelClass,
  value,
  onChange,
}: {
  id: string;
  label: string;
  labelClass: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className={labelClass}>
        {label}
      </Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 font-mono text-xs"
        autoComplete="off"
      />
    </div>
  );
}
