---
title: Agentes, versões e execuções
summary: O que é publicado, o que é fixado e o que fica registrado.
section: start
tags: agente, versão, execução, rastro, publicação
order: 1
---

## Um agente é um texto, não um programa

Um agente é escrito em português, em blocos: o objetivo, o que ele pode usar,
o que ele nunca faz. Não há código dentro dele e não há caixa de arrastar e
soltar. O que está escrito é o que a plataforma lê.

Isso é uma escolha, e ela tem uma consequência que aparece no dia a dia: quem
escreve o agente é quem entende do processo, não quem entende de programação.

## Uma versão é um congelamento

Toda vez que uma instrução é publicada, a plataforma guarda o texto inteiro e
calcula uma versão a partir dele. Publicar duas vezes o mesmo texto não cria
duas versões — a versão *é* o conteúdo.

Isso responde a pergunta que aparece meses depois: **com quais instruções
aquilo rodou?** Não com as instruções de hoje. Com as daquele dia, que ainda
estão guardadas.

## Uma execução é fixada à versão em que começou

Uma execução carrega a versão que valia quando ela abriu, e a mantém até
terminar. Publicar uma correção no meio do expediente não muda o que já está
rodando.

Isso evita a situação em que alguém aprova uma ação, a instrução muda, e a ação
executada é outra. O que foi aprovado e o que foi executado são a mesma coisa,
sempre.

## Tudo é lido de um registro só

A plataforma não guarda "o estado do agente" em lugar nenhum. Ela guarda os
passos, em ordem, cada um selado contra o anterior. O estado é o que se obtém
lendo os passos do começo ao fim.

Na prática: nenhuma tela mostra um resumo que alguém possa ter atualizado por
fora. O que a tela mostra e o que o auditor lê saem da mesma leitura.

Um passo registrado nunca é alterado nem apagado — o banco recusa. Uma correção
é um passo novo, e a correção também fica no registro.

O que impede uma execução de fazer algo é assunto de
[O que a plataforma para antes de acontecer](what-the-gate-stops.md).
