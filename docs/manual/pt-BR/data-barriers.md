---
title: Labels e barreiras de dados
summary: Como labels de empresa, área e origem não confiável acompanham uma execução, e o que a plataforma recusa.
section: security
tags: labels de dados, barreira de empresa, barreira de área, contexto, untrusted, proveniência
order: 12
---

## Labels acompanham o dado

Quando uma execução começa, a plataforma sela a empresa e a área do agente no
primeiro passo. Quando a execução veio de fora — Slack, webhook, input de
evento ou resposta de ferramenta — as marcas da origem viajam junto.

A parte importante é que os labels vivem ao lado da referência de conteúdo,
não dentro da prosa que o modelo escreveu. Um modelo não remove isso ao
resumir, e uma pessoa não concede isso ao aprovar uma única chamada de
ferramenta.

## A barreira é a regra de escopo

Uma execução só pode carregar dados de escopos que ela alcança.

| Label do dado | Escopo da execução | Resultado |
|---|---|---|
| `area:acme/platform` | `acme/platform` | permitido |
| `area:acme/platform` | `acme` | permitido |
| `area:acme/platform` | `acme/finance` | bloqueado |
| `area:acme/platform` | `cora/platform` | bloqueado |

É a mesma regra de containment usada nos grants: instalação alcança todas as
empresas, uma empresa alcança suas áreas, e uma área nunca alcança a irmã.

## Filtro de query não é barreira

Uma busca como `company = acme` reduz o que aquela requisição retorna. Ela não
prova que o dado vai continuar ali depois que o modelo ler.

A barreira é o label carregado. Depois que `area:acme/platform` entra na
execução, o Gate recusa ações em uma execução fora daquele escopo. Se um evento
abriria um listener em outra área, a execução nem é aberta.

## A publicação pega os casos irreversíveis

Antes de salvar uma versão, o FuseOne também compara os steps declarados do
rascunho com o catálogo de ferramentas. Se dado de uma fonte não confiável pode
chegar a uma ferramenta destrutiva ou financeira, a publicação é recusada.
Essas ações não ficam seguras por serem aprovadas depois que o modelo já as
propôs.

Writes reversíveis são diferentes. Eles ainda podem ser publicados, mas o Gate
em runtime pede uma pessoa na chamada concreta, com os argumentos reais na
frente dela. Isso mantém agentes úteis possíveis sem permitir que fluxos
financeiros ou destrutivos partam de texto não confiável.

## O que o operador vê

Na trilha, uma chamada bloqueada nomeia a regra `data_barrier` e explica que a
execução carrega dados de fora desta empresa ou área. Isso não é uma política
para editar e não é aprovação faltando; significa que o fluxo de dado cruzou
uma fronteira que a plataforma não tem autorização para cruzar.

Para conferir proveniência, leia os labels nos próprios passos. Um passo
`planned` mostra os labels do input enviado ao modelo naquela chamada de
planejamento. `gate_decided`, `approval_requested` e `tool_called` mostram os
labels do input proposto ou executado pela ferramenta. Um resultado de
`$fuseone.context.read` também nomeia a run de origem e o digest do artefato
compartilhado.

Se isso acontecer:

1. Confira se o agente foi publicado na empresa e área corretas.
2. Confira se a conversa do canal aponta para a mesma área do agente.
3. Confira a ligação de eventos antes de assumir que o listener está errado.
4. Não tente resolver abrindo a query. O label já viajou.

## Por que compartilhamento de contexto depende disso

[Compartilhamento de contexto entre agentes](context-sharing.md) passa
artefatos por referência, com labels, digest e origem. Passar texto livre no
prompt não basta: copia palavras e perde a autoridade ligada ao conteúdo.

A regra que precisa continuar verdadeira é simples: um listener só pode receber
contexto quando seu escopo pode carregar os labels daquele contexto, ou quando
uma autorização explícita de cruzamento de fronteira estiver registrada.
