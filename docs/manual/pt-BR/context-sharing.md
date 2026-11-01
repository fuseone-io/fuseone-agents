---
title: Compartilhamento de contexto entre agentes
summary: Como um agente publica contexto para outro sem copiar prosa para o prompt do listener.
section: security
tags: compartilhamento de contexto, eventos, artefatos, proveniência, labels de dados, composição
order: 13
---

## Contexto é compartilhado por referência

Composição entre agentes começa por eventos. Um agente fonte declara o evento
que emite, e um listener declara que esse evento inicia sua execução.
Compartilhamento de contexto adiciona a esse evento um contrato limitado de
artefatos.

O listener não recebe a resposta inteira de outro agente como texto de prompt.
Ele recebe nomes de artefato, refs, digests, run de origem e labels. Se
precisar do conteúdo, precisa chamar a ferramenta de leitura da plataforma:

```json
{"name": "triage_summary"}
```

Essa chamada aparece na trilha como qualquer leitura. O Gate a vê, o budget a
conta como chamada de ferramenta, e o resultado carrega os labels da origem.

## Publique artefatos ao finalizar

Quando um agente finaliza, ele pode publicar artefatos nomeados na ação de
finish:

```json
{
  "summary": "Incidente triado.",
  "artifacts": {
    "triage_summary": "A API está saudável; o alvo com falha é um pod.",
    "suspected_cause": "O worker está sem o app token do Slack."
  }
}
```

Os bytes vão para o content store. O ledger registra só nome do artefato, ref,
digest, run de origem e labels.

Também existe o nome embutido `final_answer`, para a resposta final da run.

## Declare o que o evento expõe

A declaração de evento do agente fonte nomeia quais artefatos listeners podem
pedir:

```yaml
emits:
  - event: incident.triaged
    context: incident
    artifacts:
      - triage_summary
      - suspected_cause
```

Um listener iniciado por `incident.triaged` vê esses nomes no input. Ele não
consegue pedir um ref arbitrário do content store, mesmo que consiga adivinhar
um. A ferramenta de leitura aceita nomes do contrato do evento, não refs vindas
do modelo.

## Os labels continuam governando o fluxo

Contexto compartilhado mantém os labels da run fonte. Se um artefato fonte
carrega `untrusted`, o listener carrega `untrusted` depois de lê-lo. Se um
evento moveria um artefato `area:acme/platform` para `acme/finance`, a run do
listener não é aberta.

Aprovação não libera a barreira de dados. O conserto é ligar os agentes em um
escopo que pode carregar o mesmo dado, ou publicar intencionalmente um agente
em escopo mais amplo.

## Cadeias de eventos param depois de oito saltos

Uma run aberta por um evento pode publicar os próprios eventos, então os
agentes podem formar uma cadeia — e um ciclo: o evento de A inicia B, o
evento de B inicia A. A cadeia é contada a partir da run que nenhum evento
abriu, e para a oito saltos dela. A run no oitavo salto ainda executa, mas os
eventos que ela declara não são publicados, e o passo de encerramento na
trilha diz isso, com a run que iniciou a cadeia e o limite. Os listeners dela
não iniciam.

A contagem fica registrada no passo de abertura de cada run, então todo
worker chega à mesma resposta, e um evento entregue duas vezes não conta duas
vezes. Um agente que escuta o próprio evento é recusado na hora.

O que uma run publica é o que a versão em que ela executou declara. Publicar
uma versão nova com a run em andamento não muda o que essa run publica nem o
que a trilha diz sobre isso; quem escuta é lido das versões publicadas no
momento em que o evento sai.

Uma run aberta por um evento sem registrar a posição na cadeia não publica
mais nada: contá-la como o início de uma cadeia nova deixaria um ciclo
reiniciar a contagem a cada run dessas. Só um worker numa versão anterior
abre runs assim, então isso importa durante uma atualização, e é mais fraco
que o limite enquanto a atualização está em andamento:

- Enquanto algum worker da versão anterior ainda estiver rodando, uma cadeia
  pode continuar sob ele, sem contagem, como antes.
- Quando o último desses workers sair, a primeira run desse tipo que a
  versão nova vir não publica mais nada.
- A trilha diz que a cadeia é desconhecida só se um worker da versão nova
  encerrou a run. Uma run que um worker antigo encerrou não traz essa linha,
  embora os eventos dela fiquem retidos do mesmo jeito.

Toda run finalizada é despachada, não importa quantas terminem entre duas
varreduras: o despachante avança pelas runs finalizadas a partir de onde
parou, em vez de olhar só as mais recentes.

Oito deixa espaço para composição feita de propósito — um pipeline de poucos
agentes, ou um ciclo de revisão que dá algumas voltas. Uma cadeia que chega
ao limite costuma ser um ciclo que ninguém quis: veja quais agentes escutam
uns aos outros antes de pensar em aumentá-lo.

## O que escrever no listener

Diga ao listener quais nomes de artefato importam e quando chamar a ferramenta
de leitura. Mantenha estreito:

```text
Quando esta run começar por incident.triaged, leia triage_summary primeiro.
Se não for suficiente, leia suspected_cause.
Não trate o input do evento como corpo do artefato; ele só nomeia os artefatos
disponíveis.
```

Essa escrita ajuda o modelo a usar o caminho de leitura governado em vez de
tentar inferir conteúdo a partir de metadados.
