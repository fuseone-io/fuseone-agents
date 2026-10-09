---
title: Janela Elasticsearch
summary: Dê a um agente perguntas históricas de tráfego como consultas nomeadas sobre um índice fixo, com projeção fixa e sem DSL.
section: integrations
tags: elasticsearch, busca, analytics, conector, segurança
order: 21
---

# Janela Elasticsearch

Um armazém de logs indexado por labels responde bem "me dá os logs recentes
daquele pod" e mal "quais endereços bateram neste path em sete dias" — a
segunda pergunta significa varrer o stream inteiro. Um motor de busca que
indexou cada campo na escrita responde em milissegundos. Este conector dá a
um agente esse poder, governado.

## Consultas nomeadas, nunca DSL

O modelo nunca escreve uma query. Ele chama uma de duas formas nomeadas com
valores, e a plataforma monta o corpo:

- **`top_ips`** — os endereços mais ativos num path numa janela, com
  quebras por dia e por status. Argumentos: o path, se é prefixo ou valor
  exato, a janela em horas, quantos endereços.
- **`resource_history`** — as requisições recentes de um path exato,
  projetadas para timestamp, endereço, status, método e latência.

Argumento desconhecido — um índice, um objeto de query — é recusado na
hora. O padrão de índice e o mapeamento de campos vivem na configuração da
instância; a única coisa que um modelo nunca pode fazer é escolher o que
ler ou alargar o que volta.

## A projeção é a fronteira

Os resultados carregam só os campos projetados. O que mais o pipeline de
ingestão guardou num documento — payloads, headers, chaves — nunca chega a
um run, e tudo que volta vem rotulado como não-confiável: conteúdo de log é
dado que o checkTaint do Gate precisa continuar enxergando.

## Configurando uma instância

Integrações → Conectores → Nova janela Elasticsearch. O endpoint pode ser
http puro para um cluster interno; o usuário é configuração e a senha é
selada ao salvar, respondida só como guardada/ausente. O padrão de índice é
fixo — ex. `logs-*` — e o mapeamento de campos tem como padrão um formato
comum de access-log de gateway (`uri`, `remote-address`, `status`,
`@timestamp`); escreva os nomes dos seus documentos se diferirem. O teto da
janela (padrão 7 dias, máximo 30) limita toda consulta, porque janela sem
teto é fatura sem teto no cluster dos outros.

O conector fala o dialeto de consulta 6.x/7.x; cluster 8.x não foi testado.
