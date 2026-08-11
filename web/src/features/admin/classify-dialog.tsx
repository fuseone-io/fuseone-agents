import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { useClassifyTool, type Effect, type Tool } from "@/features/admin/api";

const EFFECTS: { value: Effect; label: string; hint: string }[] = [
  { value: "read", label: "Leitura", hint: "Consulta dados. Não muda nada." },
  { value: "write", label: "Escrita", hint: "Altera algo, e é reversível." },
  { value: "destructive", label: "Destrutivo", hint: "Apaga ou substitui de forma difícil de desfazer." },
  { value: "financial", label: "Financeiro", hint: "Move dinheiro." },
];

/**
 * The Curator's act, and the only way write access enters the platform.
 *
 * It is a dialog rather than an inline control on purpose: promoting a tool is
 * a decision somebody signs, and the reason is recorded next to it.
 */
export function ClassifyDialog({ tool, onClose }: { tool: Tool | null; onClose: () => void }) {
  const [effect, setEffect] = useState<Effect>("read");
  const [untrusted, setUntrusted] = useState(true);
  const [reason, setReason] = useState("");
  const classify = useClassifyTool();

  if (!tool) return null;

  async function submit() {
    if (!tool) return;
    try {
      await classify.mutateAsync({ toolId: tool.toolId, effect, untrusted, reason });
      toast.success(`${tool.toolId} classificada como ${effect}`);
      onClose();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Não foi possível registrar");
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="font-mono text-base">{tool.toolId}</DialogTitle>
          <DialogDescription>
            Fica registrado com seu nome na trilha administrativa. Corrigir depois
            cria um novo registro; nenhum registro é apagado.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="effect">O que esta ferramenta faz com o mundo</Label>
            <Select value={effect} onValueChange={(v) => setEffect(v as Effect)}>
              <SelectTrigger id="effect">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {EFFECTS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label} — {option.hint}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
            <div>
              <Label htmlFor="untrusted">Traz dado de fora</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                Ler marca a execução. Uma escrita depois disso para para uma pessoa decidir.
              </p>
            </div>
            <Switch id="untrusted" checked={untrusted} onCheckedChange={setUntrusted} />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="reason">Por quê</Label>
            <Input
              id="reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="registra nota interna no CRM"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={() => void submit()} disabled={classify.isPending}>
            Registrar classificação
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
