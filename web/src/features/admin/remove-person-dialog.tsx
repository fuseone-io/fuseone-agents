import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useRemovePerson, type Person } from "@/features/admin/people-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Confirms removing somebody, by name. It stays open on a refusal, so the
 * reason is read beside the person it is about.
 */
export function RemovePersonDialog({
  person,
  onClose,
}: {
  person: Person;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const remove = useRemovePerson();

  const submit = () =>
    remove.mutate(person.id, {
      onSuccess: () => {
        toast.success(t("people.removed", { name: person.display }));
        onClose();
      },
      onError: (error) =>
        toast.error(t("people.removeFailed"), {
          description: problemMessage(error, t),
        }),
    });

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("people.removeTitle", { name: person.display })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("people.removeExplains", { name: person.display })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {remove.error && (
          <p role="alert" className="text-sm text-destructive">
            {problemMessage(remove.error, t)}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={remove.isPending}
            onClick={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            {t("people.remove")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
