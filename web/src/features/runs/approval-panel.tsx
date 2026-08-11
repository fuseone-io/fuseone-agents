import { Hand } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
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
import type { PendingApproval } from "@/lib/api/client";
import { useDecideApproval } from "./api";
import { explainRule } from "@/lib/gate-rules";

/**
 * The approval control for a suspended run.
 *
 * atSeq travels with the decision so a tab left open overnight cannot answer
 * an approval that a later step already superseded — the server rejects the
 * mismatch with a 409 rather than applying it to the wrong action.
 */
export function ApprovalPanel({
  runId,
  approval,
}: {
  runId: string;
  approval: PendingApproval;
}) {
  const decide = useDecideApproval(runId);

  const submit = (approved: boolean) => {
    decide.mutate(
      { approved, atSeq: approval.atSeq },
      {
        onSuccess: () =>
          toast.success(approved ? "Ação aprovada" : "Ação recusada"),
        onError: (error) =>
          toast.error("Não foi possível registrar a decisão", {
            description: error instanceof Error ? error.message : undefined,
          }),
      },
    );
  };

  return (
    <section
      aria-labelledby="approval-heading"
      className="rounded-xl border border-warning/40 bg-warning-surface p-4"
    >
      <div className="flex items-center gap-2">
        <Hand className="size-4 text-warning" aria-hidden />
        <h2 id="approval-heading" className="font-medium">
          Aguardando sua decisão
        </h2>
      </div>

      <p className="mt-2 text-sm">
        O agente quer executar{" "}
        <code className="rounded bg-background px-1.5 py-0.5 font-mono text-xs">
          {approval.tool}
        </code>
      </p>

      {explainRule(approval.rule) && (
        <p className="mt-1 text-sm text-muted-foreground" title={approval.reason}>
          {explainRule(approval.rule)}
        </p>
      )}

      <div className="mt-4 flex gap-2">
        <ConfirmButton
          label="Aprovar"
          title="Aprovar esta ação?"
          description={`A ferramenta ${approval.tool} será executada e o efeito ficará registrado na trilha em seu nome.`}
          disabled={decide.isPending}
          onConfirm={() => submit(true)}
        />
        <Button
          variant="outline"
          size="sm"
          disabled={decide.isPending}
          onClick={() => submit(false)}
        >
          Recusar
        </Button>
      </div>
    </section>
  );
}

function ConfirmButton({
  label,
  title,
  description,
  disabled,
  onConfirm,
}: {
  label: string;
  title: string;
  description: string;
  disabled: boolean;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" disabled={disabled}>
          {label}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancelar</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>{label}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
