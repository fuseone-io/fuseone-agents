import { useState } from "react";
import { useQuery, type QueryKey } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * RemoveButton for a resource with a revision.
 *
 * The revision is read when the confirmation opens, every time it opens, and
 * the removal answers it. Read when the page loaded, it would let somebody
 * confirm a removal of what they were looking at an hour ago, over a change
 * they never saw. Nothing can be confirmed until the read has answered.
 */
export function RevisedRemoveButton({
  title,
  description,
  disabled = false,
  revision,
  onConfirm,
}: {
  title: string;
  description: string;
  disabled?: boolean;
  revision: { queryKey: QueryKey; read(): Promise<string> };
  onConfirm: (etag: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const current = useQuery({
    queryKey: [...revision.queryKey, "revision-for-removal"],
    queryFn: revision.read,
    enabled: open,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const ready = open && current.isSuccess && !current.isFetching;

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={title}
          disabled={disabled}
        >
          <Trash2 className="size-4" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {current.isFetching && <Skeleton className="h-4 w-1/2" />}
        {current.isError && !current.isFetching && (
          <div className="flex items-center gap-2 text-sm text-danger">
            <span>{problemMessage(current.error, t)}</span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void current.refetch()}
            >
              {t("common.retry")}
            </Button>
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            disabled={disabled || !ready}
            onClick={() =>
              current.data !== undefined && onConfirm(current.data)
            }
          >
            {t("common.remove")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
