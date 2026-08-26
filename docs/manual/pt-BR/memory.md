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

## Memória proposta pelo agente

Um agente não escreve memória ativa por padrão. Aprendizado de memória é um
opt-in na versão publicada do agente; mudar essa política cria uma versão nova
e aparece na revisão de publicação.

```yaml
memory_learning:
  mode: review
  ttl_days: 30
```

V1 é modo de revisão. O agente pode chamar `$fuseone.memory.suggest` com uma
asserção estruturada: tipo, assunto, assinatura e afirmação. A plataforma
registra a sugestão com os labels e a evidência da run, mas `memory.find` ainda
não a devolve. Uma pessoa com permissão de publicação no escopo precisa aceitar
ou descartar a sugestão pela página de Memória.

Quando aprendizado de memória está ligado, a plataforma também oferece
`$fuseone.memory.find`. O agente deve usá-la antes de sugerir memória para um
caso que talvez já esteja lembrado. O caminho de sugestão também consulta a
memória ativa pelo mesmo tipo, assunto e assinatura, então um fato lembrado não
continua criando itens de revisão só porque o modelo propôs outra redação.

Sugestões em modo de revisão não pedem uma segunda aprovação antes de entrar
nessa fila. A fila de revisão é o ponto de aprovação. A sugestão ainda carrega
os labels da run, e uma policy explícita, capacidade ausente ou violação de
barreira de dados ainda pode pará-la.

V2 é modo de auto-confirmação. A mesma sugestão estruturada precisa ser
observada na quantidade configurada de runs distintas antes de virar memória
ativa. A contagem é derivada pela plataforma. Repetir a mesma sugestão dentro
de uma run não torna a memória mais forte. O modelo não envia confiança e não
escolhe empresa, área ou namespace de agente.

Sugestões em auto-confirmação são efeitos de escrita no caminho normal do
Gate. Se a observação veio de dado não confiável, ela é rebaixada para revisão
naquela sugestão: a run pode colocá-la na fila sem uma segunda aprovação, mas
uma pessoa precisa aceitá-la antes de virar memória ativa.

```yaml
memory_learning:
  mode: auto_confirm
  min_observations: 3
  ttl_days: 30
```

Nos dois modos, sugestões ficam separadas da memória ativa até que a regra de
promoção seja satisfeita. Uma sugestão pendente é material de revisão, não fato
lembrado. Se a evidência de origem é apagada antes da revisão, a sugestão é
marcada como origem apagada e não pode ser promovida.

Use modo de revisão primeiro para agentes que escrevem, aprovam, desabilitam
contas ou tocam produção. Auto-confirmação combina com memórias diagnósticas de
baixo risco, onde repetição já é evidência útil e uma pista errada ainda passa
pelo Gate normal antes de qualquer efeito.

## Corrigir memória ativa

Memória ativa é corrigida, não editada em silêncio. A página de Memória permite
que uma pessoa reescreva a afirmação mantendo o mesmo escopo, namespace de
agente, tipo, assunto, assinatura, evidência, labels e expiração. A correção
precisa de um motivo e é registrada como um novo evento de memória.

Se a própria condição lembrada foi chaveada errado, desative essa asserção e
registre outra com a assinatura certa. Mudar a assinatura significa mudar o que
runs futuras vão buscar, então deve ser uma nova asserção.

## Evidência pode expirar ou ser apagada

Memória segue retenção e apagamento. Se a evidência por trás de uma asserção é
apagada, a asserção é marcada como origem apagada e deixa de ser lembrada como
memória ativa.

Isso é deliberado. A plataforma não mantém uma afirmação útil viva depois que
o registro que a justificava se foi. O histórico de eventos continua append-only
até a retenção removê-lo, então um auditor ainda consegue ver por que a
asserção mudou de estado.

## Busca e tamanho da resposta

A busca de memória em runtime é indexada para buscas por trecho em assunto,
assinatura e afirmação. Buscas amplas ainda retornam apenas um conjunto
limitado.

A ferramenta de memória também tem um orçamento de bytes para a resposta.
Quando as asserções encontradas não cabem, a resposta diz quantas foram
omitidas pelo orçamento. As asserções continuam armazenadas; o agente pode
fazer uma consulta mais estreita por tipo, assunto ou assinatura se a primeira
resposta não for específica o bastante.

As métricas do worker reportam quantidade de leituras de memória, latência,
asserções retornadas e asserções omitidas. Essas métricas deliberadamente não
incluem nome de agente, escopo, texto de busca, id de asserção ou afirmação em
labels.

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
