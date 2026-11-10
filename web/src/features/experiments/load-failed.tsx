import { useTranslation } from "react-i18next";
import { CircleAlert } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

/** What failed to load, and the one thing to do about it. Never a stack trace. */
export function LoadFailed({ message, onRetry }: { message: string; onRetry: () => void }) {
  const { t } = useTranslation();

  return (
    <Alert variant="destructive">
      <CircleAlert className="size-4" aria-hidden />
      <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
        <span>{message}</span>
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t("experiments.retry")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
