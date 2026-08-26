---
title: Reconhecimento de efeito duplicado
summary: Como a plataforma pula o mesmo efeito entre execuções sem reutilizar resposta de ferramenta nem contornar o Gate.
section: operations
tags: dedupe, duplicado, efeito, curador, gate, trilha
order: 17
---

## Reconhecimento de duplicado não é cache

Reconhecimento de efeito duplicado impede um agente de fazer o mesmo efeito
externo duas vezes. Ele não é cache de resultado.

Cache diz: a plataforma já tem esta resposta de ferramenta, então pode
reaproveitar a resposta e evitar chamar o sistema remoto. Reconhecimento de
duplicado diz: o efeito já aconteceu, então a plataforma pula o efeito em vez
de fazê-lo de novo.

Essa distinção aparece na trilha. Um passo duplicado é registrado como decisão
do Gate com veredito de duplicado. Ele não afirma uma nova chamada de
ferramenta, e não reaproveita uma resposta anterior como se a ferramenta tivesse
respondido de novo.

## O Gate ainda vê a proposta

Nenhum efeito contorna o Gate. O engine resolve o estado de duplicado antes do
Gate, passa esse estado para a requisição do Gate, e o Gate registra o
veredito.

Isso significa que uma run ainda explica o que teria acontecido. Se uma run
contaminada propõe a mesma escrita que outra run já completou, a trilha pode
mostrar a decisão de duplicado sem fingir que a escrita foi permitida pela
política.

Duplicado é um veredito próprio. Ele não é bloqueio de política, e não conta
como uma ação bloqueada normal. Se o modelo continuar propondo o mesmo
duplicado, a run pode parar depois de pulos repetidos em vez de girar para
sempre.

## Configure a chave semântica no Curador

Reconhecimento de duplicado é ligado na classificação de uma ferramenta. O
curador escolhe quais caminhos dos argumentos definem o efeito e por quanto
tempo a janela vale.

Para uma ferramenta de criação de issue no GitHub, uma chave semântica poderia
ser:

```text
owner
repo
title
```

Isso torna `trace_id`, timestamps e outros campos por run irrelevantes. A
plataforma não cai para hash do corpo inteiro de argumentos, porque esse atalho
faria ruído inocente derrotar o reconhecimento.

Se um caminho declarado está ausente, a chamada não ganha fingerprint de
duplicado. Dado ausente é erro, não valor vazio.

## Escopo não é configurável

Empresa, área, agente e id da ferramenta sempre fazem parte da chave de
duplicado. O modelo e o curador não conseguem removê-los.

Isso impede que reconhecimento de duplicado vire canal lateral. Uma chamada em
uma empresa não pode pular uma chamada em outra empresa só porque os argumentos
visíveis parecem iguais.

A versão do agente intencionalmente não faz parte da chave. Duas versões do
mesmo agente propondo o mesmo efeito ainda estão propondo o mesmo efeito no
mundo externo.

## Pendente e confirmado são coisas diferentes

Quando uma run está prestes a executar um efeito governado com dedupe, ela
reserva a chave. Só uma run é dona da reserva. Outras runs que encontram a
reserva esperam pouco e tentam de novo.

A chave só vira confirmada depois que a ferramenta retorna com sucesso. Se a
ferramenta falha, a reserva é liberada. Se a confirmação falha depois que o
efeito aconteceu, a reserva expira depois; a plataforma prefere o risco de
tentar de novo a fingir que um efeito não registrado está feito para sempre.

Quando um duplicado confirmado é encontrado, a trilha aponta para a run e o
passo de origem quando essa origem é conhecida. Idempotência antiga por run
pode não ter origem; o duplicado continua real, mas a plataforma não inventa um
ponteiro.

## Escolha janelas por operação

A janela faz parte da decisão de governança:

| Operação | Janela típica |
|---|---|
| Criar issue de incidente | horas ou dias |
| Enviar notificação | minutos ou horas |
| Rotacionar credencial | por campanha de rotação |
| Desabilitar principal | longa ou permanente |
| Reiniciar workload | curta, ou desligado |

Não ligue reconhecimento de duplicado onde repetir é o comportamento esperado.
Se a operação correta é "enviar um lembrete por dia", a chave precisa incluir
o dia ou a feature deve ficar desligada.

## Use com memória, não no lugar de memória

Reconhecimento de duplicado resolve repetição de efeitos. Memória governada
resolve reutilização de conhecimento.

Use reconhecimento de duplicado quando a pergunta é "esta ação já aconteceu
para este assunto?" Use [memória governada](memory.md) quando a pergunta é "o
que aprendemos com evidências anteriores?"
