---
title: Slack e canais
summary: Como eventos chegam, quem autoriza uma run e por que menção, watched message e aprovação são caminhos diferentes.
section: integrations
tags: slack, canal, socket mode, event subscriptions, approval, runAs, thread
order: 4
---

## Um canal tem caminhos diferentes

Slack usa palavras parecidas para coisas diferentes. Configure cada uma no
lugar certo.

| Caminho | O que usa | Para que serve |
|---|---|---|
| Socket Mode | App-level token `xapp-...` | Receber eventos por WebSocket de saída, sem URL pública |
| Event Subscriptions | `/hooks/channel/<nome>/slack/events` | Receber menções e mensagens |
| Interactivity | `/hooks/channel/<nome>/slack` | Receber cliques em botões, como aprovação |
| API do bot | Bot token `xoxb-...` | Listar canais, postar mensagens e responder threads |
| Verificação HTTP | Signing secret | Conferir que a chamada HTTP veio do Slack |

O signing secret não substitui o app token. O app token não substitui o bot
token. Em Socket Mode, o Slack entrega eventos pelo `xapp-...`, mas postar e
listar canais ainda usa o `xoxb-...`.

## HTTP callback

Use HTTP quando o Slack consegue alcançar uma URL pública da instalação.

No Slack App:

1. Em Event Subscriptions, informe
   `https://<host>/hooks/channel/<nome>/slack/events`.
2. Assine `app_mention` para menções.
3. Assine `message.channels` se quiser watched messages em canais públicos.
4. Em Interactivity & Shortcuts, informe
   `https://<host>/hooks/channel/<nome>/slack`.
5. Convide o bot para o canal.

Se o Slack disser que a URL não respondeu ao `challenge` e o log mostrar
`POST /hooks/channel/<nome>/slack status=400`, a URL de eventos foi configurada
no endpoint de botões. O endpoint de eventos termina em `/events`.

## Socket Mode

Use Socket Mode quando a instalação não tem URL pública de entrada. O worker
abre uma conexão de saída para o Slack.

Você ainda precisa:

- app token `xapp-...` com `connections:write`;
- bot token `xoxb-...`;
- bot dentro do canal;
- eventos assinados no Slack App.

Aprovações por botão continuam precisando do caminho HTTP de Interactivity. Se
o Slack não consegue chamar `/hooks/channel/<nome>/slack`, a plataforma não
deve mostrar botões de aprovação no Slack.

## Menções

No modo "mentions only", uma pessoa menciona o bot e começa a mensagem com o
id ou nome do agente:

```text
@FuseOneAgent troubleshooting-sre entenda esse alerta
```

A conversa decide o escopo. O texto não escolhe company ou area. O agente só
inicia se a versão publicada declara trigger de conversa e existe naquele
escopo.

## Watched messages

Watched messages servem para mensagens de um sistema conhecido, como
Alertmanager ou Grafana OnCall. A autoridade não vem da mensagem. Ela vem da
configuração da conversa:

- qual agente iniciar;
- qual principal é o `run as`;
- quais Slack user ids, bot ids ou app ids podem disparar.

Use ids estáveis, não display names. Display names mudam; ids são o que o
Slack assina.

## Thread context

Se "include thread context" estiver ligado, mensagens anteriores da thread
entram como input não confiável. Isso ajuda um agente a ler o alerta original
quando a menção acontece depois.

Essa opção também inclui mensagens escritas por outras pessoas. Elas podem ser
enviadas ao provedor de modelo configurado. Ligue só em canais onde essa
consequência foi aceita.

## Checklist de diagnóstico

Se o agente não iniciou:

1. O log mostra `an ask arrived`? Se não, o evento não chegou ao FuseOne.
2. A conversa está no modo certo: menção, watched messages ou ambos?
3. O agente publicado tem trigger de conversa?
4. O agente está publicado no mesmo escopo da conversa?
5. Em watched messages, o Slack source id está permitido?
6. O `run as` existe e tem grant no escopo?
7. Em Socket Mode, o app token está salvo e o worker está conectado?
