---
title: Memória governada
summary: Como agentes reutilizam conhecimento entre execuções sem transformar texto lembrado em autoridade escondida.
section: security
tags: memória, asserções, proveniência, labels de dados, retenção, apagamento
order: 16
---

## Memória não é prosa lembrada

A memória do FuseOne guarda asserções estruturadas. Ela não guarda um parágrafo
para o modelo obedecer depois.

Uma asserção de memória nomeia tipo, assunto, assinatura e afirmação. Também
guarda evidência, observações, contagem de confirmações e labels de origem. O
modelo pode buscar essas asserções, mas não escolhe qual empresa, área ou
namespace de agente é lido. Esse escopo vem da plataforma.

Essa diferença importa. Um parágrafo lembrado pode virar instrução escondida.
Uma asserção estruturada é dado com proveniência, labels e prazo.

## O que agentes podem pedir

Um agente usa a ferramenta de leitura de memória da plataforma. Ele pode pedir
campos como:

```json
{
  "kind": "incident_signature",
  "subject": "checkout-api",
  "signature": "loki:error:datasource-timeout",
  "limit": 3
}
```

A resposta é estruturada. Ela inclui observações encontradas, contagens de
confirmação e estado da origem. Ela não entrega um ref arbitrário do content
store para o modelo, e não deixa o modelo atravessar uma fronteira de escopo.

Memória compartilhada usa as mesmas regras de escopo do
[compartilhamento de contexto entre agentes](context-sharing.md). Uma asserção
compartilhada em uma área fica dentro dessa área, a menos que a plataforma crie
explicitamente um namespace de escopo mais amplo.

## Labels viajam com a memória

Memória não transforma dado externo em dado confiável. Se uma asserção veio de
Slack, GitHub, Loki ou outra fonte externa, seus labels viajam com a leitura da
memória.

Isso significa que ler memória pode marcar a run atual como `untrusted`, `pii`
ou com um label de área. A próxima escrita é avaliada pelo Gate normalmente.
Uma leitura de memória seguida de uma ação destrutiva pode ser parada antes de
a ação sair do worker.

Isso é mecânico, não uma instrução de prompt. O modelo não é só orientado a
tratar memória com cuidado; a run carrega os labels e o Gate os lê.

## Quem deve escrever memória

Use memória para fatos estáveis que foram revisados ou confirmados por
evidência repetida:

```json
{
  "kind": "incident_signature",
  "subject": "checkout-api",
  "signature": "loki:error:datasource-timeout",
  "claim": "Geralmente causado por token expirado no datasource do Grafana",
  "confirmed": 7
}
```

Memória boa é estreita e verificável. Ela ajuda a próxima run a escolher a
primeira consulta, o dono provável ou um caminho de correção conhecido.

Evite memória para:

- instruções que o modelo deve obedecer;
- segredos ou valores secretos;
- fatos pontuais que precisam ser buscados de novo;
- opiniões amplas como "este sistema não é confiável";
- aprovações, permissões ou decisões que pertencem ao Gate.

## Evidência pode expirar ou ser apagada

Memória segue retenção e apagamento. Se a evidência por trás de uma asserção é
apagada, a asserção é marcada como origem apagada e deixa de ser lembrada como
memória ativa.

Isso é deliberado. A plataforma não mantém uma afirmação útil viva depois que
o registro que a justificava se foi. O histórico de eventos continua append-only
até a retenção removê-lo, então um auditor ainda consegue ver por que a
asserção mudou de estado.

## O que esperar

Memória reduz investigação repetida. Ela não garante que o caso de hoje é
igual ao de ontem.

Espere que agentes usem memória como pista: começar pela assinatura conhecida,
checar a evidência e então agir pelo caminho normal do Gate. Se uma asserção
lembrada carrega labels externos, escritas ainda podem exigir aprovação ou
parar.

Se o objetivo é apenas evitar abrir o mesmo chamado duas vezes, use
[reconhecimento de efeito duplicado](duplicate-effects.md). Memória é para
conhecimento reutilizável; reconhecimento de duplicado é para não repetir o
mesmo efeito.
