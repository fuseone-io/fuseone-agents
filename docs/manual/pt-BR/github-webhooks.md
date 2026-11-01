---
title: Webhooks do GitHub
summary: Inicie um agente quando o GitHub mandar um evento — um pull request aberto, uma issue rotulada —, com a entrega assinada pelo GitHub e filtrada pelo gatilho.
section: integrations
tags: github, webhook, gatilho, assinatura, eventos, entrega
order: 23
---

# Webhooks do GitHub

Um agente pode ser iniciado pelo próprio GitHub: um pull request aberto, uma
issue rotulada, um card movido. O GitHub não manda um segredo como faz quem
chama um webhook comum. Ele **assina** cada entrega com esse segredo, identifica
a entrega num cabeçalho próprio e manda todos os eventos que o hook assina. Um
gatilho de webhook com `from: github` é verificado desse jeito.

## Declare o gatilho

Na definição do agente — ou no console, em **Gatilhos**, escolhendo
**Enviado por: GitHub** — o webhook diz de onde vêm as entregas e quais delas
iniciam uma execução:

```yaml
triggers:
  - type: webhook
    path: github/reviewer
    from: github
    events: [pull_request.opened, pull_request.synchronize, issues.labeled]
```

- `evento` aceita todas as ações do evento: `issues` inicia uma execução quando
  a issue é aberta, editada ou rotulada.
- `evento.ação` aceita só aquela ação: `pull_request.labeled`.
- Pelo menos um evento é obrigatório. Sem a lista, toda entrega do GitHub
  iniciaria uma execução, o que num repositório movimentado vira conta.
- `ping` não entra na lista: o GitHub manda uma vez, quando o hook é criado, e
  a plataforma responde sem iniciar nada.

A publicação recusa um webhook do GitHub sem eventos, um nome que não tem a
forma `evento` ou `evento.ação` e eventos num webhook que não diz
`from: github`.

**Só a forma do nome é conferida.** Um evento escrito errado que ainda tem a
forma certa — `pull_requset.opened` — é publicado e nunca dispara. Se um agente
não começa quando você espera, abra **Recent Deliveries** no webhook do GitHub:
uma entrega que o gatilho não nomeou aparece com 200 e
`{"status":"ignored"}`.

## Gere a chave

Um caminho declarado recusa tudo até ter uma chave. Na página do agente,
**Webhooks** lista o caminho com a marca **GitHub**; **Gerar uma chave** mostra
a chave uma única vez.

A chave de um webhook do GitHub fica selada com a chave mestra da instalação
(`FUSEONE_MASTER_KEY`), porque a plataforma precisa lê-la de novo para conferir
cada assinatura. Sem chave mestra, o console se recusa a gerar a chave e o
webhook continua fechado. Um webhook comum guarda só o resumo criptográfico da
chave.

A chave nunca é aceita em `X-FuseOne-Secret`: ela assina, não é enviada.

## Configure o webhook no GitHub

Em **Settings → Webhooks** do repositório ou da organização, ou nas
configurações de webhook de um GitHub App:

| Campo | Valor |
|---|---|
| Payload URL | O endereço da instalação seguido de `/hooks/` e do caminho, por exemplo `https://agents.example.com/hooks/github/reviewer` |
| Content type | `application/json` |
| Secret | A chave que o console mostrou |
| Events | Os eventos que o gatilho lista, ou mais: a plataforma ignora o resto |

Um corpo em formato de formulário é recusado com o motivo: a entrada da
execução precisa ser o JSON que o GitHub mandaria.

## O que cada entrega recebe de volta

| A entrega | Resposta | Execução |
|---|---|---|
| Assinada, com um evento que o gatilho lista | 202 | Aberta |
| O mesmo `X-GitHub-Delivery` de novo | 200, com a mesma execução | Nenhuma nova |
| Assinada, com um evento ou ação fora da lista | 200 | Nenhuma |
| `ping` | 200 | Nenhuma |
| Sem assinatura, assinatura errada, caminho sem chave, caminho desconhecido ou corpo acima de 1 MiB | 401, com as mesmas palavras para todos | Nenhuma |
| Assinada sem `X-GitHub-Delivery`, ou com um corpo que não é JSON | 400 ou 415, com o motivo | Nenhuma |

Uma resposta fora de 2xx faz o GitHub tentar de novo e marcar o hook como com
falha. Por isso as entregas que o gatilho não quer são confirmadas, e não
recusadas. Tudo o que não se provou recebe a mesma resposta, para que quem
testa caminhos não descubra quais existem.

## O que a execução recebe

A entrada da execução é a entrega, embrulhada para o agente saber o que ela é —
o corpo do GitHub não traz o nome do próprio evento:

```json
{
  "event": "pull_request",
  "action": "opened",
  "delivery": "72d3162e-cc78-11e3-81ab-4c9367dc0958",
  "payload": { "…": "o corpo que o GitHub mandou" }
}
```

A execução começa **não confiável**: a assinatura prova que o GitHub mandou a
entrega, não que o texto da issue ou do pull request dentro dela seja seguro
para agir. Num repositório público qualquer pessoa escreve esse texto, então o
Gate trata o que o agente faz depois de lê-lo de acordo — veja
[Labels e barreiras de dados](data-barriers.md).
