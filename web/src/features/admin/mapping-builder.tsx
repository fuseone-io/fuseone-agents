import { useTranslation } from "react-i18next";
import { Plus, ShieldCheck, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MappingField } from "@/features/admin/mapping-field";
import type { GroupMapping } from "@/features/admin/identity-api";
import { CompanyAreaFields } from "@/features/scope/company-area-fields";

const ROLES: GroupMapping["role"][] = [
  "admin",
  "author",
  "approver",
  "curator",
  "auditor",
];

/**
 * Which group gets which role, where. The group is the identity provider's
 * and is typed; where is chosen from what is registered. The installation,
 * `*`, comes from its own button.
 */
export function MappingBuilder({
  mappings,
  onChange,
}: {
  mappings: GroupMapping[];
  onChange: (mappings: GroupMapping[]) => void;
}) {
  const { t } = useTranslation();
  const update = (index: number, patch: Partial<GroupMapping>) =>
    onChange(mappings.map((m, i) => (i === index ? { ...m, ...patch } : m)));

  return (
    <div className="flex flex-col gap-2">
      {mappings.length === 0 && (
        <p className="text-xs text-muted-foreground">
          {t("identity.noMapping")}
        </p>
      )}

      {mappings.map((mapping, index) => (
        <div
          key={index}
          className="grid grid-cols-[1fr_1fr_1fr_130px_32px] items-end gap-2"
        >
          <MappingField
            label={t("identity.group")}
            value={mapping.group}
            onChange={(group) => update(index, { group })}
            id={`group-${index}`}
          />
          <CompanyAreaFields
            value={mapping}
            shape={{ idPrefix: `mapping-${index}`, labelClass: "text-xs" }}
            onChange={(scope) => update(index, scope)}
          />

          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`role-${index}`} className="text-xs">
              {t("identity.role")}
            </Label>
            <Select
              value={mapping.role}
              onValueChange={(role) =>
                update(index, { role: role as GroupMapping["role"] })
              }
            >
              <SelectTrigger id={`role-${index}`} className="!h-9 w-full">
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
            onClick={() => onChange(mappings.filter((_, i) => i !== index))}
          >
            <X className="size-4" aria-hidden />
            <span className="sr-only">
              {t("identity.removeMapping", { n: index + 1 })}
            </span>
          </Button>
        </div>
      ))}

      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          onClick={() =>
            onChange([
              ...mappings,
              { group: "", company: "", area: "", role: "author" },
            ])
          }
        >
          <Plus className="size-4" aria-hidden />
          {t("identity.addMapping")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          onClick={() =>
            onChange([
              ...mappings,
              { group: "", company: "*", area: "", role: "admin" },
            ])
          }
        >
          <ShieldCheck className="size-4" aria-hidden />
          {t("identity.addAdminMapping")}
        </Button>
      </div>
    </div>
  );
}
