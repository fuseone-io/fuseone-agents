import { useTranslation } from "react-i18next";
import { Plus, X } from "lucide-react";
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
import type { GroupMapping } from "@/features/admin/identity-api";

const ROLES: GroupMapping["role"][] = [
  "author",
  "approver",
  "curator",
  "auditor",
];

/**
 * Which group gets which role, where.
 *
 * A provider with no mapping grants nothing, and that is the correct default:
 * authenticating proves who somebody is, and it should never by itself decide
 * what they may do. The rows say so in the order somebody thinks it — this
 * group, in this area, may do this.
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
          <Field
            label={t("identity.group")}
            value={mapping.group}
            onChange={(group) => update(index, { group })}
            id={`group-${index}`}
          />
          <Field
            label={t("identity.company")}
            value={mapping.company}
            onChange={(company) => update(index, { company })}
            id={`company-${index}`}
          />
          <Field
            label={t("identity.area")}
            value={mapping.area}
            onChange={(area) => update(index, { area })}
            id={`area-${index}`}
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

      <Button
        variant="outline"
        size="sm"
        className="h-8 self-start"
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
    </div>
  );
}

function Field({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-xs">
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
