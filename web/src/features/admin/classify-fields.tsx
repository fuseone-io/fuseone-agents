import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { UndoPicker } from "@/features/admin/undo-picker";
import type { Effect, Tool } from "@/features/admin/api";

/** The four things a tool can do to the world, most restrictive last. */
const EFFECTS: Effect[] = ["read", "write", "destructive", "financial"];

export type Ruling = {
  effect: Effect;
  untrusted: boolean;
  reason: string;
  compensatedBy: string;
};

/**
 * What the Curator is deciding: what the tool does, whether its results can be
 * trusted, and what takes an act by it back.
 *
 * The three are one judgement, which is why they are one form. Ruling that a
 * tool moves money and leaving how to reverse it for another screen is how an
 * installation ends up unable to undo the calls it most needs to.
 */
export function ClassifyFields({
  ruling,
  onChange,
  tools,
  self,
}: {
  ruling: Ruling;
  onChange: (ruling: Ruling) => void;
  tools: Tool[];
  self: string;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <Label htmlFor="effect">{t("admin.whatItDoes")}</Label>
        <Select
          value={ruling.effect}
          onValueChange={(v) => onChange({ ...ruling, effect: v as Effect })}
        >
          <SelectTrigger id="effect">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {EFFECTS.map((effect) => (
              <SelectItem key={effect} value={effect}>
                {t(`effect.${effect}`)} — {t(`admin.effectHint.${effect}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
        <div>
          <Label htmlFor="untrusted">{t("admin.bringsOutside")}</Label>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("admin.readMarksRun")}
          </p>
        </div>
        <Switch
          id="untrusted"
          checked={ruling.untrusted}
          onCheckedChange={(v) => onChange({ ...ruling, untrusted: v })}
        />
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="compensatedBy">{t("admin.whatUndoesIt")}</Label>
        <UndoPicker
          tools={tools}
          self={self}
          value={ruling.compensatedBy}
          onChange={(v) => onChange({ ...ruling, compensatedBy: v })}
        />
        <p className="text-xs text-muted-foreground">
          {t("admin.undoTakesTheResult")}
        </p>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="reason">{t("admin.why")}</Label>
        <Input
          id="reason"
          value={ruling.reason}
          onChange={(e) => onChange({ ...ruling, reason: e.target.value })}
          placeholder={t("admin.reasonPlaceholder")}
        />
      </div>
    </div>
  );
}
