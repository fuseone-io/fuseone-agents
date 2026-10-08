---
title: Aprovações permanentes
summary: Pré-aprove uma ferramenta exata para um agente, com nome, teto e prazo, para a madrugada não esperar um clique.
section: operations
tags: aprovação, permanente, mandato, autonomia, taint
order: 20
---

# Aprovações permanentes

Uma aprovação permanente é uma aprovação humana dada de antemão: uma pessoa
pré-aprova **uma ferramenta exata** para **um agente** em **um escopo**, com
**teto diário** e **prazo**. Quando um run parquear aguardando aprovação e
um mandato o cobrir, a plataforma decide o park na hora — com o nome do dono
do mandato e o id dele na decisão, exatamente como se tivesse clicado. A
trilha do run diz "Approved by" aquela pessoa, porque aquela pessoa aprovou
de fato; só aprovou de dia.

## Quando conceder

Conceda quando as três forem verdade: os argumentos da ação vêm de conteúdo
que a plataforma desconfia com razão (logs, mensagens), então todo uso
esperaria um clique; o momento do uso é quando ninguém está acordado; e **a
própria ferramenta carrega guardas estruturais** que tornam o pior insumo
malicioso sobrevivível. A lista de bloqueio Cloudflare é o exemplo com essa
forma: só endereço literal, faixas protegidas recusadas em código, teto
próprio, bloqueios que expiram.

Uma aprovação permanente libera o park — não torna a ferramenta segura.
Esse julgamento é seu, e o campo de motivo obrigatório é onde você o
escreve. O `unblock_ip` da lista de bloqueio é o contra-exemplo pronto:
**não** conceda mandato para ele. Remover proteção é exatamente a ação que
um agente enganado nunca deve tomar sem supervisão, e o clique humano é o
teto dele.

## Os limites do mandato

- **Um id de ferramenta exato**, com instância. Padrões são recusados: um
  mandato precisa saber o que cobre.
- **Teto diário** (no máximo 100). Cobrir e gastar um uso são um passo
  atômico, então o teto segura sob concorrência; esgotado, os parks voltam
  a esperar clique até a meia-noite.
- **Prazo**, no máximo 90 dias. Renovar é uma re-decisão deliberada.
- **Revogação é imediata** e mantém a linha: a trilha mostra o mandato e o
  fim dele.

## Quem concede

Conceder exige permissão própria (`approval:grant`, Admin por padrão) —
aprovar uma classe de ações futuras é estritamente mais do que clicar um
card, que qualquer Approver faz. Criação, revogação e cada uso entram na
trilha administrativa.
