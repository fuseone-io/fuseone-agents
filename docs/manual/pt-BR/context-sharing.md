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
