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

Uma conversa pode nomear o agente que ela inicia. Com um agente escolhido, quem
menciona o bot não precisa nomeá-lo, e a frase inteira é a pergunta:

```text
@FuseOneAgent entenda esse alerta
```

Nessa conversa **nenhum outro agente inicia por menção**. Mencionar um agente
diferente é recusado com o nome do agente da conversa, em vez de rodar outro
agente na frase de quem perguntou.

Sem agente escolhido — a opção "nenhum — a mensagem nomeia o agente" — a
mensagem precisa começar com o id ou nome do agente, e qualquer agente
publicado no escopo da conversa pode ser iniciado:

```text
@FuseOneAgent troubleshooting-sre entenda esse alerta
```

Nos dois casos a conversa decide o escopo e o texto não escolhe company ou
area. O agente só inicia se a versão publicada declara trigger de conversa e
existe naquele escopo.

O agente escolhido **seleciona, não autoriza**. A run continua acontecendo em
nome da pessoa cuja conta Slack está vinculada a um usuário da plataforma;
quem não tem vínculo continua sendo recusado.

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
2b. A menção nomeou um agente diferente do agente escolhido na conversa?
3. O agente publicado tem trigger de conversa?
4. O agente está publicado no mesmo escopo da conversa?
5. Em watched messages, o Slack source id está permitido?
6. O `run as` existe e tem grant no escopo?
7. Em Socket Mode, o app token está salvo e o worker está conectado?
