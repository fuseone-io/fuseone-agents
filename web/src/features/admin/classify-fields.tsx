import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
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
  /*
    Unchosen is its own value, and not one of the four.

    A form whose effect starts at `read` is a form that answers for the Curator
    with the most permissive answer available — and `read` is *allowed*, so the
    zero value of the control was a grant. The empty string is not a
    classification anybody can submit; it is the state before there is one.
  */
  effect: Effect | "";
  untrusted: boolean;
  reason: string;
  compensatedBy: string;
  dedupe: DedupeRuling;
};

export type DedupeRuling = {
  enabled: boolean;
  windowSeconds: string;
  argPaths: string;
};

/**
 * What the Curator is deciding: what the tool does, whether its results can be
 * trusted, what takes an act by it back, and whether the same effect can be
 * recognised across runs.
 *
 * These are one judgement, which is why they are one form. Ruling that a
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
            {/* A real placeholder, so an unchosen effect looks unchosen. */}
            <SelectValue placeholder={t("admin.chooseAnEffect")} />
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

      <div className="flex flex-col gap-3 rounded-lg border p-3">
        <div className="flex items-start justify-between gap-4">
          <div>
            <Label htmlFor="dedupe">{t("admin.dedupeEffects")}</Label>
            <p className="mt-1 text-xs text-muted-foreground">
              {t("admin.dedupeEffectsHint")}
            </p>
          </div>
          <Switch
            id="dedupe"
            checked={ruling.dedupe.enabled}
            onCheckedChange={(enabled) =>
              onChange({ ...ruling, dedupe: { ...ruling.dedupe, enabled } })
            }
          />
        </div>

        {ruling.dedupe.enabled && (
          <div className="grid gap-3 sm:grid-cols-[minmax(0,10rem)_minmax(0,1fr)]">
            <div className="flex flex-col gap-2">
              <Label htmlFor="dedupe-window">{t("admin.dedupeWindow")}</Label>
              <Input
                id="dedupe-window"
                type="number"
                min={1}
                value={ruling.dedupe.windowSeconds}
                onChange={(e) =>
                  onChange({
                    ...ruling,
                    dedupe: {
                      ...ruling.dedupe,
                      windowSeconds: e.target.value,
                    },
                  })
                }
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="dedupe-args">{t("admin.dedupeArgPaths")}</Label>
              <Textarea
                id="dedupe-args"
                value={ruling.dedupe.argPaths}
                onChange={(e) =>
                  onChange({
                    ...ruling,
                    dedupe: { ...ruling.dedupe, argPaths: e.target.value },
                  })
                }
                placeholder={t("admin.dedupeArgPathsPlaceholder")}
                rows={3}
              />
              <p className="text-xs text-muted-foreground">
                {t("admin.dedupeArgPathsHint")}
              </p>
            </div>
          </div>
        )}
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
