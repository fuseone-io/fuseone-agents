---
title: Ler uma execução
summary: O que cada passo da trilha significa, por que uma execução terminou, e onde olhar quando algo deu errado.
section: troubleshooting
tags: trilha, execução, passo, diagnóstico, terminou, teto
order: 11
---

## A trilha é o registro, não um resumo

Cada passo é selado contra o anterior. Nada é editado depois — uma correção é um passo novo.

Isso significa que a trilha não é uma versão simplificada do que aconteceu. **É o que aconteceu**, e é a mesma leitura que um auditor faz.

## Os passos que você vai ver

| Passo | O que significa |
|---|---|
| **O modelo propôs** | O agente decidiu tentar alguma coisa |
| **O Gate decidiu** | A avaliação, com o veredito e a razão |
| **Orçamento reservado** | O custo estimado foi separado antes da chamada |
| **Ferramenta chamada** | Saiu de verdade, com os argumentos |
| **Ferramenta respondeu** | O que voltou, e se veio marcado como externo |
| **Execução terminou** | O fim, com a razão |

Argumentos e resultados não ficam no passo — o passo guarda **referência e digest**, e o conteúdo vive onde retenção e apagamento alcançam. Por isso abrir um passo é um ato deliberado, e por isso um conteúdo apagado aparece como *apagado* e não como vazio.

Quando aprendizado de memória está ligado, uma execução com entrada humana pode
começar com uma chamada de plataforma a `$fuseone.memory.find` antes do primeiro
passo **O modelo propôs**. Isso não é uma proposta de modelo faltando. É a busca
inicial de memória registrada como chamada normal de ferramenta, para que os
labels de proveniência continuem viajando pela execução.

## Por que ela terminou

É a pergunta mais comum, e a trilha responde de formas diferentes:

**Terminou normalmente** — o agente respondeu. A resposta final fica no armazenamento de conteúdo, e a trilha diz isso.

**O modelo não propôs outra ação** — ele devolveu texto em vez de chamar uma ferramenta ou a ação de finish, então a execução estacionou para inspeção. Se o texto dizia "vou prosseguir", o agente pretendia continuar e não continuou: é caso de ajustar a instrução para chamar a ferramenta agora.

**Parou esperando alguém** — está na fila humana.

**Bateu um teto** — custo, passos, tokens ou chamadas. A recusa carrega o teto, o gasto e a estimativa da chamada que cruzou.

**A investigação parou de progredir** — três chamadas canonicamente diferentes
à mesma ferramenta de leitura devolveram o mesmo resultado completo. A
plataforma estaciona antes de comprar outro turno do modelo. O passo
estacionado nomeia ferramenta, quantidade de chamadas, tamanho original,
acertos de cache e digest do resultado. Retome depois de restringir a
investigação ou conferir por que a fonte continua devolvendo a mesma evidência.

**Falha do provedor** — sobrecarga, limite de taxa, chave inválida. A causa aparece com o provedor e o código, e a tela `Runtime` mostra se está acontecendo com todo mundo.

## O custo aparece zerado

Se toda execução mostra custo zero, quase sempre é **tarifa não configurada** para aquele modelo.

O preço de mercado que aparece na tela de Custo é **referência**, em dólar, e não entra na contabilidade. A contabilidade usa só a tarifa que a instalação configurou, na própria moeda. Sem ela, o honesto é zero.

E execuções antigas não são reprecificadas: o que foi gravado como zero continua zero.

## Casos de uso

### Uma escrita parou e você não esperava

Olhe o passo do Gate: ele nomeia a regra. Se for taint, procure acima o passo em que o agente leu algo de fora — é dali que veio a marca.

### O agente ignorou a instrução

Provavelmente não ignorou. Instrução orienta o modelo, mas **`stopsWhen` de uma etapa e as ferramentas no alcance são fontes separadas** que o texto não altera. O editor avisa quando os dois se contradizem.

### A execução ficou cara

Abra os passos **O modelo propôs** e compare a composição do prompt. A trilha
separa bytes de resultados enviados ao modelo dos bytes omitidos pela
compactação, com atribuição por ferramenta. Escritas e buscas de memória
equivalentes, além de chamadas cujo desfecho ficou desconhecido depois do
reinício de um worker, são puladas pela identidade canônica. Leituras concluídas
podem consultar novamente, mas três leituras consecutivas de uma ferramenta
que produzam o mesmo resultado completo estacionam como
`investigation_stalled` antes que o teto de dinheiro seja a única parada.

O que decide cada parada está em [O que a plataforma para antes de acontecer](what-the-gate-stops.md) e em [Políticas](policies.md).
