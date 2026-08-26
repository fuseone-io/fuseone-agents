---
title: Simulação e regressões
summary: Como ensaiar um agente antes da produção, transformar casos reais em corpus e ler o resultado sem confundir ferramenta seca com run grátis.
section: operations
tags: simulação, regressão, ensaio, corpus, dry run, custo, política
order: 14
---

## Ensaio é uma chamada paga ao modelo com ferramentas secas

Uma simulação abre runs que se parecem com runs de produção na trilha. O modelo
é chamado, o Gate decide, políticas casam, tetos valem, e o resultado fica
registrado.

A camada de ferramentas fica seca: nada é enviado ao Slack, GitHub, CRM, banco
de dados ou outro sistema externo. Isso torna o ensaio seguro para efeitos,
não gratuito para uso de modelo. A prévia de custo mostra a exposição máxima
em dinheiro antes de iniciar.

Use simulação antes de publicar, antes de promover e antes de mudar uma
política de monitor para enforce.

## Escolha situações, não prompts

Uma boa situação é o que o agente vai receber de verdade:

```json
{
  "channel": "slack",
  "message": "@FuseOneAgent investigue o alerta #175979",
  "thread": [
    "alertNodeDownProdUS severity=critical",
    "namespace=payments job=engineering-ai-agents instance=172.16.109.29:8080"
  ]
}
```

Escreva situações que cobrem o formato do trabalho:

- um caso normal que deve finalizar;
- um caso que deve pedir uma pessoa;
- um caso que deve ser recusado por política;
- um caso com dado faltando;
- um caso com input ruidoso ou enganoso.

Não escreva situações como instruções ideais. Se produção envia texto de Slack,
simule texto de Slack. Se produção envia JSON de webhook, simule o payload do
webhook.

## Leia o resultado

O relatório separa três resultados:

| Resultado | O que significa |
|---|---|
| Passou | A run chegou em um fim aceitável |
| Precisa de olhar | Uma pessoa deve inspecionar a trilha antes de confiar neste caso |
| Falhou | O fim observado contradiz a expectativa |

Um bloqueio de política em simulação não é automaticamente falha. Se o caso
serve para provar que uma chamada arriscada é parada, o bloqueio é a evidência
que você queria.

Abra a trilha de qualquer caso marcado como precisa de olhar. A pergunta
importante não é se o texto final soa bom; é quais fatos o agente leu, quais
ferramentas tentou e qual regra parou ou permitiu cada ação.

## Salve um corpus de regressão

Quando a simulação cobre situações reais, salve como corpus. Um corpus é um
conjunto de casos que a plataforma pode rodar de novo depois de uma mudança.

Use um corpus para uma promessa:

- "o agente de troubleshooting no Slack trata alertas de node down";
- "o agente de resposta no GitHub nunca escreve a partir de input não confiável sem aprovação";
- "o agente de billing pede uma pessoa antes de ação destrutiva no CRM".

Corpora pequenos são mais fáceis de confiar. Dez casos que ensinam algo são
melhores que cem casos que ninguém lê.

## Quando rodar regressões

Rode o corpus quando:

- instruções mudaram;
- ferramentas foram adicionadas ou removidas;
- a classificação de uma ferramenta mudou;
- uma política mudou;
- o estágio de autonomia mudou;
- o provedor ou modelo mudou.

Se uma regressão começa a falhar depois de uma política mudar, leia primeiro o
passo do Gate. A política pode estar fazendo exatamente o que foi escrita para
fazer.

## Erros comuns

### "A simulação não postou no Slack"

Isso é esperado. Simulação mantém ferramentas secas. Para testar entrega no
Slack, rode um agente real em draft ou copilot com uma conversa estreita.

### "A simulação custou dinheiro"

Isso é esperado. O modelo foi chamado. A prévia de custo existe para uma
pessoa decidir antes de gastar.

### "O relatório está verde, mas produção parou"

Confira se produção carregou labels que o corpus não carregou. Uma menção no
Slack, corpo de webhook ou artefato de contexto pode marcar a run como não
confiável, e o Gate vai parar uma escrita que um ensaio limpo não tentou.

As regras estão em [What the platform stops before it happens](what-the-gate-stops.md)
e a trilha está explicada em [Reading a run](reading-a-run.md).
