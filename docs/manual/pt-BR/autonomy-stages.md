---
title: Rascunho, copiloto e autônomo
summary: O que muda entre os três estágios, e como decidir que um agente está pronto para o seguinte.
section: governance
tags: autonomia, rascunho, copiloto, autônomo, promover, confiança
order: 8
---

## Três degraus, não um interruptor

Todo agente publicado nasce em **rascunho** e pausado. Sair de cada degrau é uma decisão de alguém, registrada.

| Estágio | O que ele pode fazer |
|---|---|
| **Rascunho** | Não abre execução real. Só ensaio |
| **Copiloto** | Executa de verdade, e toda escrita espera uma pessoa |
| **Autônomo** | Executa e escreve sozinho, dentro do que política e classificação permitem |

Repare no que **não** muda entre eles: o Gate avalia igual nos três. Autônomo não é "sem freio" — é "sem espera humana no que já estava liberado". Ferramenta destrutiva continua bloqueada, taint continua parando escrita, teto continua valendo.

## Rascunho

O agente existe, tem versão publicada, e não abre execução real.

É onde se escreve, ensaia e corrige. O ensaio roda contra situações que você escolhe, com as ferramentas secas — nada é enviado nem alterado em sistema externo.

**Saia daqui quando:** o ensaio passa nas situações que importam, e as que ele marcou como *precisa de olhar* você entendeu e resolveu.

## Copiloto

Executa de verdade. Toda escrita para e espera uma pessoa aprovar aquela chamada específica.

É o degrau onde se aprende o que o agente realmente faz com dado real — que nunca é exatamente o que o ensaio mostrou.

**Saia daqui quando:** as aprovações viraram rotina sem surpresa. Não é uma contagem mágica, mas três perguntas:

- **As aprovações estão sendo iguais?** Se você aprova a mesma coisa toda vez sem pensar, a decisão já não é uma decisão.
- **Alguma recusa te surpreendeu?** Se sim, o agente ainda está aprendendo o trabalho — ou a política está errada.
- **Você olharia a trilha de uma execução dessas e ficaria confortável?** Se a resposta depende de quem estava olhando, ainda não.

## Autônomo

Executa e escreve sem esperar aprovação, **dentro do que já estava permitido**.

O que continua parando: efeito destrutivo e financeiro, conteúdo marcado como vindo de fora, política que escala ou nega, e qualquer teto.

**Isso significa que promover a autônomo não é o momento de relaxar a política — é o momento em que ela passa a ser a única coisa segurando.** Antes, um humano revisava cada escrita e pegaria o que a política deixou passar. Agora não.

## Como promover com segurança

A ordem que funciona:

1. **Ensaie** até as situações relevantes passarem.
2. **Publique em rascunho** e tire do pause.
3. **Copiloto**, e aprove de verdade — lendo os argumentos, não clicando por hábito.
4. **Antes de ir para autônomo**, escreva as políticas que segurariam o que te preocupa, e ligue em **monitorar**. Veja se elas bateriam.
5. **Troque as políticas para impor**, e só então promova.

O passo 4 é o que costuma ser pulado, e é o que separa promover de apostar.

## Voltar é normal

Descer um degrau não é fracasso — é o mecanismo funcionando. Um agente que passou a errar depois de uma mudança no sistema do outro lado volta para copiloto até se entender o que mudou.

O que a política decide está em [Políticas](policies.md), e o que o Gate para independente de estágio está em [O que a plataforma para antes de acontecer](what-the-gate-stops.md).
