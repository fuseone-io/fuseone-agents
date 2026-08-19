import { useTranslation } from "react-i18next";
import { ClipboardPenLine } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

export const MAX_CAPTURE_CHARS = 12_000;

export function InterviewCapture({
  value,
  onChange,
  onReview,
}: {
  value: string;
  onChange: (value: string) => void;
  onReview: () => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="rounded-xl border bg-card shadow-sm">
      <div className="flex items-center gap-3 border-b border-border-subtle px-4 py-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
          <ClipboardPenLine className="size-4" aria-hidden />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{t("interview.capture")}</h2>
          <p className="text-xs text-muted-foreground">
            {t("interview.captureHint")}
          </p>
        </div>
      </div>

      <div className="flex flex-col gap-3 p-4">
        <Label htmlFor="interview-capture">{t("interview.describe")}</Label>
        <Textarea
          id="interview-capture"
          value={value}
          maxLength={MAX_CAPTURE_CHARS}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t("interview.describePlaceholder")}
          className="min-h-[220px] resize-y text-sm leading-6"
        />
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="font-mono text-2xs tabular-nums text-muted-foreground">
            {t("interview.characters", {
              count: value.length,
              max: MAX_CAPTURE_CHARS,
            })}
          </p>
          <Button onClick={onReview} disabled={value.trim() === ""}>
            {t("interview.reviewAnswers")}
          </Button>
        </div>
      </div>
    </section>
  );
}
