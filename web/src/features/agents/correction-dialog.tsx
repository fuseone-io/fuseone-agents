import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Mono } from "@/components/shared/mono";
import { correctionOptions } from "@/features/agents/correction-options";
import { useRecordRegression } from "@/features/agents/regressions-api";
import type { SimulationCase } from "@/features/agents/simulation-api";

/**
 * "This one came out wrong, and here is what should have been true."
 *
 * The options are read from what the case did rather than asked for from
 * nothing: a blank form is a form only somebody who already knows the
 * vocabulary can fill, and the author has just finished reading the case.
 *
 * What is recorded is checkable, because it is re-run against every future
 * version (FU-12). The note is for the person who reads it in a year, and is
 * never what the battery checks.
 */
export function CorrectionDialog({
  agentId,
  entry,
  onClose,
}: {
  agentId: string;
  entry: SimulationCase;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const record = useRecordRegression(agentId);
  const options = correctionOptions(entry);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [note, setNote] = useState("");

  const toggle = (key: string) =>
    setChosen((was) => {
      const next = new Set(was);
      if (!next.delete(key)) next.add(key);
      return next;
    });

  const submit = () =>
    record.mutate(
      {
        runId: entry.runId ?? "",
        note: note || undefined,
        expectations: options
          .filter((o) => chosen.has(o.key))
          .map((o) => o.expectation),
      },
      {
        onSuccess: () => {
          toast.success(t("correction.recorded"), {
            description: t("correction.recordedHint"),
          });
          onClose();
        },
        onError: (error) =>
          toast.error(t("correction.failed"), {
            description: error instanceof Error ? error.message : undefined,
          }),
      },
    );

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("correction.title")}</DialogTitle>
          <DialogDescription>{t("correction.explains")}</DialogDescription>
        </DialogHeader>

        <ul className="flex flex-col gap-2">
          {options.map((option) => (
            <li key={option.key} className="flex items-center gap-2.5">
              <Checkbox
                id={option.key}
                checked={chosen.has(option.key)}
                onCheckedChange={() => toggle(option.key)}
              />
              <Label htmlFor={option.key} className="font-normal">
                {t(option.label)}
                {option.tool && (
                  <Mono className="ml-1.5 text-xs">{option.tool}</Mono>
                )}
                {option.expectation.step && (
                  <span className="ml-1.5 text-xs text-muted-foreground">
                    {t("correction.atStep", { step: option.expectation.step })}
                  </span>
                )}
              </Label>
            </li>
          ))}
        </ul>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="correction-note">{t("correction.note")}</Label>
          <Input
            id="correction-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t("correction.notePlaceholder")}
          />
          {/* Said plainly, because a note that looked like a rule would be a
              correction nobody is checking. */}
          <p className="text-xs text-muted-foreground">
            {t("correction.noteHint")}
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={chosen.size === 0 || record.isPending}
          >
            {t("correction.record")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
