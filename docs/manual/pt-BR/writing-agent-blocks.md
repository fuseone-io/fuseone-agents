---
title: Como escrever bons blocos
summary: Instruções que fazem o agente agir agora, parar no lugar certo e não prometer trabalho futuro.
section: authoring
tags: blocos, instruções, agente, exemplo, stopsWhen, ferramentas
order: 3
---

## O bloco é o contrato do agente

Um agente não recebe intenção escondida. Ele recebe blocos de texto. O melhor
bloco diz o que fazer, quais evidências buscar, quando encerrar e o que nunca
deve acontecer.

O texto deve ser operacional: uma pessoa que executa o trabalho hoje precisa
reconhecer o processo ali.

## A ação de finish termina a execução

Quando o modelo devolve texto sem chamar uma ferramenta ou a ação de finish, a
execução estaciona para inspeção. Então evite frases como "vou consultar", "vou
continuar" ou "farei a análise". Se a próxima ação exige uma ferramenta, o
agente deve chamar a ferramenta naquele turno. Se o trabalho terminou, ele deve
chamar a ação de finish.

Escreva isso no bloco:

```text
Não anuncie trabalho futuro. Se precisar consultar logs, métricas ou outro
sistema, chame a ferramenta agora. Chame a ação de finish só quando a análise
estiver concluída ou quando souber explicar por que não pode continuar.
```

## Purpose

Purpose é uma frase sobre o resultado. Não coloque a sequência inteira aqui.

Bom:

```text
Diagnosticar um alerta recebido no Slack e responder na thread com causa
provável, evidências consultadas e próximos passos seguros.
```

Ruim:

```text
Usar Grafana, procurar logs, olhar métricas, talvez ver wiki e responder.
```

O ruim mistura objetivo, ferramentas e talvez. Talvez é lacuna, não instrução.

## How to act

How to act é o roteiro. Use nomes estáveis: ids de datasource, labels,
campos e filtros. Display names mudam; ids e labels são o que as ferramentas
entendem.

Exemplo para SRE:

```text
1. Leia a mensagem e o contexto da thread. Identifique alertname, aplicação,
namespace, pod, instância, cluster, severidade e horário aproximado.
2. Consulte métricas no Grafana usando o datasource Mimir Query
   (id 7-JKlf87k). Prefira filtros por namespace_name, pod_name,
   container_name, job ou instance.
3. Consulte logs no Grafana usando o datasource Loki (id bDvEKnCnk) com os
   mesmos filtros. Nunca comece por uma query ampla.
4. Compare a evidência de métricas e logs. Se uma ferramenta falhar por
   datasource inválido ou vazio, liste os datasources e tente de novo com o id
   correto.
5. Responda na thread com diagnóstico, evidências e próximos passos.
```

## When to stop

When to stop não é "quando terminar". Diga quais condições encerram a run.

```text
Pare quando tiver evidência suficiente para apontar causa provável e próximos
passos, ou quando não houver identificador mínimo para consultar com segurança.
Se não conseguir achar aplicação, namespace, pod, instância ou horário, diga
qual dado faltou e pare.
```

Isso evita duas falhas: gastar tokens em busca genérica e finalizar como se a
análise estivesse completa quando não estava.

## Never

Never é guardrail. Escreva ações proibidas, não preferências vagas.

```text
Nunca use queries amplas em Loki ou Mimir. Nunca consulte runbook ou Outline
para este agente. Nunca diga que vai continuar a investigação: continue usando
ferramentas agora ou pare com uma razão clara.
```

Never não remove ferramenta do alcance. Se o agente não deve poder chamar uma
ferramenta, tire a ferramenta do pack ou da superfície do servidor. Texto é
instrução; o Gate é mecanismo.

## Exemplo completo

```text
Purpose
Diagnosticar alertas recebidos no Slack e responder na thread com causa
provável, evidências e próximos passos.

How to act
Leia a mensagem e o contexto da thread. Identifique alertname, aplicação,
namespace, pod, instância, cluster, severidade e horário aproximado.

Use o Grafana para consultar Mimir Query (datasource id 7-JKlf87k) e Loki
(datasource id bDvEKnCnk). Use filtros específicos: namespace_name, pod_name,
container_name, job, instance ou labels equivalentes. Se um id de datasource
falhar ou voltar vazio, liste os datasources e tente novamente com o id
correto.

Cruze métricas e logs antes de concluir. Se encontrar causa provável, cite as
evidências. Se não encontrar, diga exatamente quais consultas foram feitas e
qual informação faltou.

When to stop
Pare quando tiver causa provável e próximos passos, ou quando faltar dado
mínimo para consultar sem query ampla.

Never
Nunca use queries amplas em Loki ou Mimir. Nunca consulte runbook ou Outline.
Nunca anuncie trabalho futuro: se precisar consultar algo, chame a ferramenta
agora; se não puder, responda por que parou.
```

## Checklist antes de publicar

- O bloco diz o resultado esperado.
- O roteiro nomeia ferramentas, ids e filtros quando eles importam.
- Existe uma condição clara de parada.
- Never proíbe comportamentos concretos.
- O agente não promete continuar depois de escrever uma resposta.
- O pack de ferramentas combina com o texto. Se o texto proíbe uma ferramenta,
  remova a ferramenta do alcance também.
