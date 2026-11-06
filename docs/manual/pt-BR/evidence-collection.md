---
title: Coleta de evidências
summary: Colete eventos de uma fonte declarada e analise em agregado, com a cobertura de cada número informada.
section: integrations
tags: evidence, segurança, resposta a incidentes, conector, cobertura
order: 25
---

# Coleta de evidências

Uma investigação pergunta à telemetria quanto saiu, para onde e se algo aconteceu.
Responder exige mais eventos do que um modelo consegue ler, e um número sem cobertura
é uma resposta errada que parece certa: uma fonte que bateu no teto, recusou a
consulta ou nunca registra o evento sempre parece uma contagem pequena.

Este conector coleta de uma fonte que você declara e responde em agregado. O modelo
nunca vê um evento e nunca escreve uma consulta.

## As três ferramentas

Cada instância dá ao agente três ferramentas, todas de leitura:

- **`check`** valida a credencial e roda o controle positivo da fonte. Rode primeiro.
- **`collect`** roda uma consulta nomeada sobre uma janela e devolve um manifesto e
  uma referência da evidência. Pede aprovação se a sua política mandar.
- **`analyze`** conta, soma e agrupa o que este run coletou. Também aceita uma janela
  menor e um filtro por campo.

Uma coleta longa demais para uma chamada responde `partial`, com a referência da
evidência. Chame `collect` de novo com essa referência em `resume` para continuar.

## Como ler a resposta

Toda resposta informa primeiro a janela em UTC, o escopo, as contagens e a cobertura.

| Cobertura | Significado |
|---|---|
| `complete` | Todos os eventos foram lidos |
| `partial` | A chamada esgotou o orçamento. Retome |
| `floor` | Bateu num teto, ou a fonte parou de avançar. O número é um piso |

O `analyze` então afirma só o que isso sustenta:

| Resultado | Significado |
|---|---|
| `observed` | Há eventos que batem. Prova o que a fonte registrou, não a intenção |
| `not_observed` | Nada bateu, todos os eventos foram lidos, o controle passou e nenhum evento ficou sem medição. Não significa que não aconteceu |
| `not_observable` | Nada bateu e falta algo acima. Ninguém consegue afirmar |

Uma soma informa quantos eventos não conseguiu contar e quantos vieram zerados,
porque um campo que certos tipos de evento nunca trazem não é zero.

## O que ele recusa

- **Valor fora do padrão**, e qualquer valor com aspas, barra invertida ou caractere
  de controle, seja qual for o padrão.
- **Consulta sem um filtro.** Parâmetro nunca é opcional, e um filtro que não acha
  nada nunca cai para um mais amplo.
- **Evidência de outro run.** A evidência é selada e pertence ao run que coletou.
- **Janela no `analyze` fora do que foi coletado.**

Fonte que recusa a consulta ou nega a credencial é falha, nunca zero eventos.

## Como declarar uma fonte

No console, abra **Integrações → Connectors** e escolha **Nova instância → Nova fonte
de evidências**. Dê um nome e um escopo à instância. Comece por um dos dois exemplos
abaixo, ou cole uma declaração em **Colar JSON**, e confira os campos no console da sua
fonte. Quem garante a leitura é a credencial, então use um token sem escopo de escrita.
Ele fica selado e nunca volta para a tela.

O nome da instância entra no id da ferramenta: uma instância chamada `gateway` dá ao
agente `evidence.gateway.check`, `evidence.gateway.collect` e
`evidence.gateway.analyze`.

A mesma declaração também pode ir pela API de administração:
`PUT /admin/integrations/connectors/instances/{name}` com `"connector": "evidence"`, o
escopo, o token em `token` e a declaração em `evidence`.

O primeiro exemplo tem períodos relativos fixos, paginação por offset e recusa dentro
de uma resposta 200. Ele marca como sem medição os eventos que o gateway não conseguiu
decriptar, e toda consulta precisa manter esse marcador.

```json
{
  "baseUrl": "https://tenant.example.com",
  "authHeader": "Netskope-Api-Token",
  "control": "all_pages",
  "rows": "result",
  "refusal": { "field": "ok", "value": "0" },
  "gap": { "field": "bypass_traffic", "value": "yes" },
  "maxEvents": 200000,
  "paging": { "kind": "offset", "limitParam": "limit", "pageSize": 5000, "offsetParam": "offset" },
  "window": { "kind": "period", "periodParam": "timeperiod", "periods": [3600, 86400, 604800, 2592000, 7776000] },
  "queries": [
    {
      "id": "all_pages",
      "description": "Every page event, unfiltered. The positive control.",
      "path": "/api/v2/events/data/page",
      "fields": ["_id", "bypass_traffic"],
      "idField": "_id"
    },
    {
      "id": "pages_by_user",
      "description": "One user's page events, with bytes.",
      "path": "/api/v2/events/data/page",
      "filters": [{ "name": "query", "value": "user eq '{user}'" }],
      "parameters": [{ "name": "user", "pattern": "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+" }],
      "fields": ["_id", "timestamp", "user", "hostname", "domain", "app", "appcategory", "client_bytes", "server_bytes", "bypass_traffic"],
      "idField": "_id",
      "timeField": "timestamp"
    },
    {
      "id": "uploads_by_user",
      "description": "One user's uploads, with file names and no bytes.",
      "path": "/api/v2/events/data/application",
      "filters": [{ "name": "query", "value": "user eq '{user}' and activity ~ 'Upload'" }],
      "parameters": [{ "name": "user", "pattern": "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+" }],
      "fields": ["_id", "timestamp", "user", "app", "activity", "object", "object_type", "file_size", "bypass_traffic"],
      "idField": "_id",
      "timeField": "timestamp"
    }
  ]
}
```

O segundo tem intervalos absolutos e paginação por cursor. `find_agent` é uma busca
sem janela; `analyze` com `groupBy: ["id"]` devolve o id do agente para a consulta
seguinte.

```json
{
  "baseUrl": "https://console.example.com",
  "authHeader": "Authorization",
  "authPrefix": "ApiToken ",
  "control": "all_activities",
  "rows": "data",
  "paging": { "kind": "cursor", "limitParam": "limit", "pageSize": 1000, "cursorParam": "cursor", "nextCursor": "pagination.nextCursor" },
  "window": { "kind": "range", "startParam": "createdAt__gte", "endParam": "createdAt__lte", "format": "iso" },
  "queries": [
    {
      "id": "all_activities",
      "description": "Every console activity, unfiltered. The positive control.",
      "path": "/web/api/v2.1/activities",
      "fields": ["id"],
      "idField": "id"
    },
    {
      "id": "find_agent",
      "description": "Finds a device by its exact name, to get its agent id.",
      "path": "/web/api/v2.1/agents",
      "filters": [{ "name": "computerName", "value": "{host}" }],
      "parameters": [{ "name": "host", "pattern": "[A-Za-z0-9._-]{1,64}" }],
      "fields": ["id", "computerName", "osName", "agentVersion", "lastActiveDate", "networkStatus", "siteName"],
      "idField": "id",
      "windowless": true
    },
    {
      "id": "activities_by_agent",
      "description": "One agent's console activities, including logons.",
      "path": "/web/api/v2.1/activities",
      "filters": [{ "name": "agentIds", "value": "{agent}" }],
      "parameters": [{ "name": "agent", "pattern": "[0-9]{1,32}" }],
      "fields": ["id", "createdAt", "activityType", "primaryDescription", "secondaryDescription"],
      "idField": "id",
      "timeField": "createdAt"
    }
  ]
}
```

São pontos de partida, e os nomes são exemplos. Rode `check` antes de confiar: uma
fonte pode responder um período que não aceita com um sucesso vazio, e o controle que
falha é como você descobre.

Não declare endpoint de exportação que guarda posição no servidor. O GET dele move o
stream, e outro leitor perde eventos.

## Limites

Uma coleta guarda até 200.000 eventos por padrão e 1.000.000 no máximo. Os eventos
ficam no armazenamento de conteúdo, sob a retenção do run. Só há suporte a fontes por
`GET`, e consulta que roda como job assíncrono ainda não é suportada.
