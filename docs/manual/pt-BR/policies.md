---
title: Políticas
summary: Escrever uma regra que o Gate avalia em todo passo, testá-la contra o passado e ligá-la sem susto.
section: governance
tags: política, regra, escopo, condição, monitorar, impor, negar, escalar
order: 7
---

## O que uma política é

Uma regra avaliada em **todo passo de agente**, antes de qualquer efeito acontecer. Ela tem três partes: o que ela alcança, quando ela vale, e o que acontece quando bate.

Ela não é instrução do agente. Instrução orienta o modelo e o modelo pode não seguir; política é decidida pela plataforma e não depende de ninguém obedecer.

## As três partes

### Escopo — o que a regra alcança

A ferramenta, por nome ou por `*`, e opcionalmente quais efeitos ela cobre.

Nenhum efeito selecionado cobre todos. É o padrão mais amplo possível, e vale ler duas vezes antes de salvar.

### Condição — quando ela vale

Sem condição, a regra vale para tudo que o escopo alcança. É assim que se escreve "negar toda escrita em crm".

Com condições, **todas precisam ser verdadeiras** ao mesmo tempo. Não existe "ou".

### Efeito — o que acontece

| Efeito | O que faz |
|---|---|
| **Permitir** | Registra e segue |
| **Escalar** | Para até uma pessoa decidir |
| **Negar** | Recusa e registra |

## Monitorar antes de impor

Toda política pode ser gravada em **modo monitorar**: ela é avaliada, aparece na trilha, e **não muda decisão nenhuma**.

É assim que se descobre o alcance real de uma regra sem quebrar nada. Ligue em monitorar, deixe rodar, veja onde ela bateu. Só então troque para impor.

O botão **Rodar contra o histórico** faz o mesmo sem esperar: avalia a regra contra decisões já registradas e mostra o que ela teria feito.

## A ordem entre políticas não importa

Se várias regras batem no mesmo passo, o Gate devolve **a mais restritiva**: negar vence escalar, que vence permitir.

Isso significa que você não precisa pensar em ordenação, e que **adicionar uma política nunca afrouxa nada** — com uma exceção.

**Permitir é a única coisa que afrouxa o padrão embutido**, e por isso é a que merece revisão. Use quando uma ferramenta classificada como escrita é comprovadamente segura em um contexto estreito, e prefira estreitar o escopo a alargar o efeito.

## O código e o motivo aparecem para quem foi barrado

O **código** — `POL-100` — vai para a trilha e para a mensagem de quem foi recusado.

O **motivo** é a diferença entre alguém ler *"bloqueado por POL-100"* e alguém entender o que fazer a respeito. Escreva a razão, não a regra: *"respostas ao cliente saem por um canal revisado"* ensina; *"nega crm.send"* repete o que a tela já mostra.

## Casos de uso

### Nada é apagado sem uma pessoa

Escopo `*`, efeito **destrutivo** apenas, ação **escalar**. Sem condição.

Toda chamada destrutiva passa a esperar decisão humana, em qualquer ferramenta, inclusive nas que forem conectadas amanhã.

### Uma ferramenta específica nunca

Escopo `crm.delete_account`, ação **negar**. Motivo dizendo por que essa operação sai de outro lugar.

Mais estreito e mais forte que o anterior — use quando a resposta é sempre não.

### Escrita só em horário comercial

Escopo `*` com efeito **escrita**, condição de horário, ação **escalar**.

Fora do horário, escrita espera alguém. Dentro, segue o padrão.

### Descobrindo o alcance de uma regra nova

Grave em **monitorar**, rode contra o histórico, e olhe a trilha por alguns dias. Se ela bateu onde você não esperava, o escopo está largo demais — estreite antes de impor.

Como uma política é avaliada e o que ela recusa está em
[O que a plataforma para antes de acontecer](what-the-gate-stops.md).
