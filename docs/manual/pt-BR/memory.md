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

## Ensinar a partir de uma execução

Toda memória nasce de uma execução. Há dois caminhos e os dois são governados.

**"Lembrar disso", no rastro da execução.** O botão aparece no passo final de
uma execução que produziu algo citável, e só para quem pode publicar. Ele abre
um painel onde a plataforma já respondeu tudo o que sabe: o escopo e a execução
vêm do registro que está sendo lido, a citação vem do passo, os labels vêm do
rastro, e o agente é lido do ledger quando o pedido chega.

**A página de Memória.** Mesmo formulário, com uma diferença: ali a pessoa
escreve o identificador da execução, porque não há uma na tela.

Nos dois casos, o que é perguntado é o que só uma pessoa sabe:

| Perguntado | Derivado pela plataforma |
|---|---|
| tipo, assunto, assinatura | agente, a partir da execução citada |
| a afirmação | labels, dobrados do rastro até o passo citado |
| quem lê (agente ou compartilhada) | digest e passo da citação |
| o motivo | contagens iniciais e prazo de 30 dias |

### A evidência é lida, não digitada

O painel mostra execução, passo, artefato e digest, e não deixa alterar nenhum
deles. Não é uma restrição de conveniência: todos vêm do ledger, e um campo que
alguém pode mudar é um campo que alguém pode mudar para algo que a execução
nunca produziu — o servidor recusa isso, então uma caixa editável ali só levaria
a uma recusa.

Quando a execução publicou mais de uma saída citável, a pessoa escolhe entre os
**nomes que o ledger gravou**. Nunca digita um.

### Vocabulário de artefatos

Uma citação nomeia uma execução e uma de suas saídas:

- **`final_answer`** — a resposta de fechamento. É o padrão e o caso comum.
  Nenhuma execução pode publicar um artefato com esse nome; ele é reservado.
- **um artefato publicado** — saídas nomeadas que a execução compartilhou por
  referência para quem escuta o evento.
- **`memory_suggestion`** — os argumentos de uma proposta que o próprio agente
  fez. A plataforma resolve essa forma para dar proveniência a propostas
  antigas, mas o console **não** oferece "Lembrar disso" sobre ela: aquela
  proposta já está na fila de revisão, e aceitá-la é como ela vira memória.

### Os labels aparecem antes da decisão

O painel mostra os labels que a execução tinha acumulado **até o passo citado**,
não os do passo sozinho. Uma resposta limpa dentro de uma execução envenenada é
um fato que o veneno alcançou, e lembrá-lo como confiável é a inferência que o
Gate se recusa a fazer.

Enquanto o rastro ainda está carregando, o painel diz que está lendo — nunca que
não há labels. Ausente e nenhum são respostas diferentes.

## O que já responde isto

Antes de salvar, o painel diz o que a plataforma já guarda sobre aquela
identidade. Não é um bloqueio: ensinar um fato que já existe **corrige** o que
existe, e normalmente é isso que a pessoa quer. O que ela não consegue saber
pelo formulário é qual dos dois está fazendo.

| Estado | O que é oferecido |
|---|---|
| ativa | salvar corrige o texto, mantendo contagens, autoria e evidência |
| desabilitada | **reativar** — o servidor não mescla numa linha desabilitada |
| vencida | salvar com esta evidência **renova** por 30 dias |
| origem apagada | nada. É a resposta honesta |
| compartilhada cobre | **melhorar a compartilhada**, explicitamente |
| proposta pendente | nada aqui; ela é decidida na fila de revisão |

**Cobrir não é corrigir.** Uma memória compartilhada equivalente cobre uma
criação no namespace do agente, e permanece byte a byte igual. Melhorá-la é um
botão que troca o namespace do formulário — nunca algo que acontece por baixo
de uma escrita de agente.

## Prazos

Memória vale **30 dias** a partir da decisão que a escreveu. Vencida, ela não é
mais lembrada por runs, mas continua visível e continua sendo a mesma memória.

| Transição | Prazo |
|---|---|
| correção de memória ativa | preservado |
| reasserção com evidência nova | renova por 30 dias |
| auto-confirmação após novas observações | renova por 30 dias |
| reativação de uma desabilitada | renova por 30 dias |
| aceite de proposta sobre memória ativa | preservado, nunca encurta |

## Conteúdo com cara de segredo

A plataforma recusa memória que contenha chave privada ou token completo em
formato reconhecido. Nada libera essa recusa, e a recusa **nunca repete o valor
recusado** — nem em mensagem, nem em log, nem em evento.

Texto longo e aleatório o bastante para ser uma credencial gera um aviso, que
uma pessoa com permissão de publicação pode sobrepor. A sobreposição não é um
recibo: ela marca a asserção com o label `secret`, que aparece na linha, na
lista e no detalhe do evento. Uma sobreposição que ninguém consegue ver depois é
uma proteção que parou de valer em silêncio.

Na auto-confirmação não existe sobreposição, porque não existe pessoa. Uma
proposta que a plataforma não distingue de uma credencial continua pendente para
revisão, em vez de virar legível para todas as runs por ter sido feita duas
vezes.

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
`$fuseone.memory.find`. No começo de uma execução com entrada humana, o FuseOne
faz uma busca de memória registrada antes da primeira chamada ao modelo, usando
termos curtos dessa entrada. A busca continua sendo uma chamada de ferramenta:
passa pelo Gate, aparece na trilha, e quaisquer labels da memória retornada
viajam para a execução.

O agente pode chamar `memory.find` de novo depois com tipo, assunto ou
assinatura mais estreitos. O caminho de sugestão também consulta a memória
ativa pelo mesmo tipo, assunto e assinatura, então um fato lembrado não
continua criando itens de revisão só porque o modelo propôs outra redação.

A busca automática roda uma vez. Se depois o modelo propuser um
`memory.find` equivalente, mudando apenas ordem ou espaços do JSON, a
plataforma pula como a mesma chamada. Uma busca materialmente diferente ou
mais estreita continua permitida; o agente pode sair de texto amplo para tipo,
assunto ou assinatura sem que isso seja confundido com retry.

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

Esse rebaixamento acompanha a sugestão acumulada, não só a run mais recente.
Se uma observação veio de dado não confiável, observações limpas posteriores
não lavam esse label; a sugestão continua na revisão humana.

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

A busca de memória em runtime divide o texto livre em um conjunto pequeno de
termos e ranqueia correspondências entre assunto, assinatura e afirmação.
Identificadores fortes como `not_in_channel` ou `superset.alert.delivery` pesam
mais; palavras comuns ainda precisam concordar com a asserção o bastante para
ela ser retornada. Uma busca como `Slack not_in_channel` pode encontrar uma
asserção cujo assunto nomeia Slack e cuja afirmação nomeia o código de erro.
Buscas amplas ainda retornam apenas um conjunto limitado.

A busca por texto livre também tem orçamento de termos. Identificadores fortes
entram primeiro; depois entram termos comuns que não são ruído, até seis termos
distintos normalizados. Palavras comuns em português e inglês são ignoradas,
mas identificadores curtos como `s3`, `db` ou `qa` continuam buscáveis quando
aparecem como termo próprio. Quando uma chamada de runtime envia mais que isso, a
resposta nomeia os termos usados, quantos termos foram omitidos e o motivo
`search_term_budget`. Assim uma busca limitada é diferente de "não existe
memória"; o agente pode tentar de novo com identificadores mais fortes, como
código de erro, nome de sistema ou assinatura.

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
