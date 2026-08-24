---
title: Guia do auditor
summary: Como verificar uma run, inspecionar o que foi armazenado, entender apagamento e exportar evidência sem confiar em screenshot.
section: reference
tags: auditoria, ledger, verificação, exportação, content store, apagamento, evidência
order: 15
---

## O ledger é a fonte da verdade

Toda run é uma cadeia de passos. Cada passo aponta para o anterior, então um
passo alterado quebra a verificação. A tela do console, os exports e a
verificação leem a mesma cadeia.

Uma auditoria deve começar pelo id da run, não por um screenshot. Abra a run,
verifique a trilha e então inspecione os passos que importam.

## O que vive em um passo

Um passo registra fatos que a plataforma precisa manter:

- o que abriu a run;
- qual versão foi fixada;
- em nome de quem a run agiu;
- o veredito e a regra do Gate;
- medições de custo, token e tamanho de prompt;
- referências e digests para conteúdo volumoso.

O passo não deve carregar payload grande ou pessoal inline. Argumentos de
ferramenta, resultados de ferramenta, input da run, respostas finais e
artefatos de contexto compartilhado vivem no content store. O ledger registra
uma referência e um digest.

Essa distinção importa: o ledger é evidência de longa duração; o content store
é onde retenção e apagamento se aplicam.

## Verifique antes de interpretar

Use **Verify the trail** na run. A verificação recompõe a cadeia e informa o
primeiro passo que não bate.

Se a verificação falhar, pare de interpretar a run como evidência até o
problema de integridade ser resolvido. Uma cadeia quebrada significa que o
registro não diz mais o que aconteceu.

## Abrindo conteúdo selado

Abrir argumentos ou resultados é um ato deliberado porque esses bytes podem
conter dado pessoal ou conteúdo de terceiro. Quando o conteúdo ainda existe, a
tela mostra os bytes e o digest os liga ao passo. Quando o conteúdo foi
apagado, a tela diz apagado em vez de vazio.

Vazio, ausente e apagado são fatos diferentes:

| Estado | Leitura |
|---|---|
| Vazio | A run não produziu conteúdo para aquele campo |
| Ausente | O objeto referenciado não foi encontrado |
| Apagado | Retenção ou pedido de titular removeu os bytes |

Não trate uma resposta apagada como "o agente não disse nada". Ele disse algo;
os bytes é que não estão mais retidos.

## Exporte evidência

Exporte a run quando uma revisão precisa sair do console. O export é útil
porque carrega dados dos passos, referências, digests e o resultado da
verificação.

Não substitua um export por texto copiado de um resultado de ferramenta. Texto
copiado perde o digest, a decisão do Gate que permitiu aquilo e os labels que
carregava.

## Apagamento e retenção

Apagamento alcança conteúdo, não o ledger imutável. O ledger mantém o fato de
que o conteúdo existiu, com referência e digest, para a plataforma ainda
explicar a run sem reter o dado pessoal em si.

Linhas operacionais de canal têm retenção própria. Linhas que ainda devem uma
resposta são preservadas até a plataforma responder ou não ter mais uma run
entregável de onde responder.

## O que conferir em perguntas comuns

### "Quem autorizou isto?"

Leia `on behalf of` na run e o passo de aprovação, se existir. Uma run aberta
por mensagens observadas no Slack usa o principal `runAs` configurado, não o
autor da mensagem no Slack.

### "Por que esta escrita não aconteceu?"

Leia o passo do Gate. Uma barreira de dados ou ação destrutiva bloqueia de vez.
Um veredito que precisa de aprovação espera uma pessoa. Aprovação libera só a
chamada exata e os argumentos exatos que foram aprovados.

### "Isto usou contexto de outro agente?"

Procure o resultado da ferramenta `$fuseone.context.read`. A trilha nomeia o
artefato, a run de origem e o digest. O conteúdo é lido por uma ferramenta da
plataforma, não copiado para o prompt inicial do listener.

### "Posso comparar isto com logs de produção?"

Sim. Chamadas de ferramenta registram referências e horários, mas não headers
secretos ou credenciais. Compare por id da run, número do passo, nome da
ferramenta e timestamps do sistema externo.

Para diagnóstico do dia a dia, comece por [Reading a run](reading-a-run.md).
Para limites de fluxo de dados, leia [Data labels and barriers](data-barriers.md).
