---
title: Seu primeiro agente, do começo ao fim
summary: Um caminho completo — conectar ferramentas, escrever, ensaiar, publicar e promover — na ordem que funciona.
section: start
tags: começar, primeiro agente, passo a passo, tutorial, exemplo
order: 0
---

## O exemplo

Vamos montar um agente que **responde dúvidas de infraestrutura no Slack**: alguém menciona o agente num canal, ele consulta métricas e logs, e responde na thread.

É um caso bom para começar porque só lê. Você vai ver a plataforma funcionando sem arriscar nada — e depois vai ver o que muda quando ele precisar escrever.

## 1. Conecte as ferramentas

Em **Integrações**, conecte o servidor MCP que dá acesso a métricas e logs.

Ao conectar, a plataforma descobre as ferramentas que ele oferece — e **nenhuma delas funciona ainda**. Isso é deliberado, não é falha.

Detalhe em [MCPs e credenciais](mcp-and-credentials.md).

## 2. Classifique o que cada ferramenta faz

Ainda em Integrações, diga o que cada ferramenta faz ao mundo: **ler, escrever, destrutivo ou financeiro**.

A plataforma não adivinha pelo nome. Alguém precisa decidir, e essa decisão fica registrada com quem a tomou.

No nosso caso, consultar métrica e log é **ler**. Enquanto o agente só ler, ele nunca vai parar pedindo aprovação.

A escada está em [O que a plataforma para antes de acontecer](what-the-gate-stops.md).

## 3. Escreva o agente

Em **Agentes → Novo**, descreva o trabalho com suas palavras. A plataforma converte isso nas sete partes fixas, e você revisa antes de gerar o rascunho.

As partes que mais importam aqui:

- **Objetivo** — responder dúvidas de infra com base em métricas e logs.
- **Como agir** — consultar antes de responder; nunca responder de memória.
- **Nunca** — não inventar número que não veio de uma consulta.
- **Quando parar** — se a pergunta não for sobre infra, dizer isso e parar.

Escolha as ferramentas que ele alcança. **Só as que ele precisa** — alcance é capacidade, e texto dizendo "não use X" não remove X.

Como escrever bem está em [Como escrever bons blocos](writing-agent-blocks.md).

## 4. Ensaie

Antes de publicar, use **Ensaiar**. Escolha situações — perguntas reais que já apareceram servem bem — e rode.

As ferramentas ficam secas: nada é enviado nem alterado em sistema externo. Mas **as chamadas ao modelo são cobradas**, então ensaio não é grátis.

Olhe as situações marcadas como *precisa de olhar*. Se o Gate recusou alguma coisa, é isso que você queria descobrir aqui e não em produção.

## 5. Publique — e ele nasce parado

Publicar cria uma versão e **não liga nada**. O agente nasce em rascunho e pausado.

Tire do pause e deixe em **rascunho** enquanto ainda estiver ajustando.

## 6. Ligue o canal

Em **Integrações → Canais**, conecte o Slack e configure a conversa do canal onde o agente vai atender.

Para começar, use **menções**: o agente só responde quando alguém o chamar. É o modo mais previsível.

Detalhe em [Slack e canais](slack-channels.md).

## 7. Copiloto, e observe

Promova para **copiloto** e deixe rodar com gente usando.

Como o agente só lê, quase nada vai parar. O que você está observando aqui é outra coisa: **ele está respondendo bem?** Abra algumas execuções e leia a trilha — o caminho que ele fez, quantas consultas, quanto custou.

Como ler está em [Ler uma execução](reading-a-run.md).

## 8. Quando ele precisar escrever

Uma hora alguém vai querer que ele **abra um chamado** em vez de só responder.

Aí tudo muda, e é aqui que a plataforma fica interessante:

- A ferramenta de abrir chamado é **escrita**, então toda chamada para e espera aprovação.
- Se o agente leu o texto de um chamado antes, a escrita **para mesmo em autônomo** — conteúdo que veio de fora carrega marca.
- Você aprova as primeiras, lendo os argumentos, e aprende o que ele faz.

Como decidir está em [Aprovar uma ação](approving.md).

## 9. Antes de autônomo, escreva a política

Quando as aprovações viraram rotina sem surpresa, **não promova ainda**.

Primeiro escreva as políticas que segurariam o que te preocupa — por exemplo, escalar toda escrita fora do horário comercial — e ligue em **modo monitorar**. Deixe rodar e veja se elas bateriam onde você espera.

Depois troque para impor, e **só então** promova para autônomo.

Porque depois de autônomo **a política é a única coisa segurando**: não há mais um humano lendo cada escrita atrás dela.

O passo a passo dos estágios está em [Rascunho, copiloto e autônomo](autonomy-stages.md), e como escrever a regra está em [Políticas](policies.md).

## O que você aprendeu no caminho

Que a plataforma **recusa por padrão** em cada ponto: ferramenta descoberta não age, agente publicado nasce parado, escrita espera gente, conteúdo externo marca a execução.

Nenhuma dessas recusas é defeito. Cada uma é o lugar onde alguém decide — e essa decisão fica registrada.
