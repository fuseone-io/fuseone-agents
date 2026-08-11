/**
 * The Gate's `reason` is developer-facing English; `rule` is the stable key it
 * ships alongside. The console localises from the key, so the server never has
 * to know the reader's language and the trail still names what fired.
 */
export const GATE_RULES: Record<string, string> = {
  passed: "",
  capability: "A ferramenta está fora do pacote de capacidades da execução.",
  contract: "Os argumentos não batem com o contrato da ferramenta.",
  taint: "Os argumentos derivam de dado não confiável lido em um passo anterior.",
  policy: "A política da instalação exige tratamento para esse tipo de efeito.",
  budget: "A chamada ultrapassaria o teto da execução.",
  idempotency: "Esta chamada já consta na trilha; repeti-la duplicaria o efeito.",
  approval: "Liberada por decisão humana registrada nesta execução.",
};

export function explainRule(rule: string | undefined): string {
  return (rule && GATE_RULES[rule]) || "";
}
