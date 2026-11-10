---
title: Experimentos
summary: Como rodar uma versão do agente contra uma versão do dataset com outro provedor, modelo ou esforço, ler o resultado caso a caso e comparar dois experimentos sem confundir um experimento com uma bateria.
section: operations
tags: experimento, modelo, provedor, esforço, repetições, dataset, comparação, simulação
order: 27
---

## Um experimento pergunta "e se", e não muda nada

Um experimento roda uma versão de um agente contra os casos de uma versão do
dataset, com uma configuração: outro provedor, outro modelo, outro esforço, ou
nenhum desses. Ele responde a uma pergunta: esta versão iria melhor ou pior em
outro modelo?

Ele não muda nada no agente. Passar para o modelo que ganhou é publicar uma
nova versão, que tem a própria bateria. Até lá, o agente roda exatamente como
foi publicado.

Cada run de um experimento é uma [simulação](simulation-and-regressions.md): o
modelo é chamado, o Gate decide, as políticas se aplicam, os tetos valem e a
camada de ferramentas é seca. Nada chega ao Slack, ao GitHub ou a qualquer
outro sistema.

## Um experimento nunca é uma bateria

Uma bateria é o corpus rodado contra a versão como foi publicada, e é a
evidência que o portão de início e a checagem de drift leem. Um experimento
pode rodar a versão num modelo que ela nunca usaria, então ele nunca conta como
bateria, qualquer que seja a configuração, inclusive nenhuma:

- não inicia nem para uma versão;
- não conta como a última bateria da versão;
- não deixa um agente sair de Rascunho.

Para iniciar um agente, rode o corpus dele, não um experimento.

## Comece um

Abra o agente, escolha **Experimentos** no menu dele e depois **Novo
experimento**.

| Campo | O que faz |
|---|---|
| Versão do agente | A versão que vai rodar. A atual por padrão; uma mais antiga também serve. |
| Provedor | Deixe em **Como publicado** para manter o provedor da versão. |
| Modelo | Deixe vazio para manter o modelo da versão. |
| Esforço | Deixe em **Como publicado** para manter o esforço da versão. |
| Repetições | Quantas vezes cada caso roda, de 1 a 5. |

Todo campo é opcional, e um campo que você não mexe é o da versão. Um provedor
sozinho roda o modelo publicado nele. Um modelo sozinho roda no provedor
publicado. Um esforço sozinho muda só o esforço. O par resultante precisa ser
um que esta instalação consegue chamar de fato; se não for, o experimento é
recusado antes de abrir qualquer run.

O console roda a versão mais recente do dataset. Para fixar uma mais antiga,
use a API abaixo.

### Qualquer mudança roda sem o fallback

Uma versão pode declarar um modelo de fallback para quando o próprio falha
repetidamente. Um experimento com **qualquer** mudança, mesmo só do esforço,
roda sem ele: quando as tentativas do modelo escolhido acabam, o run para e o
relatório diz isso. O resultado nunca mostra um modelo enquanto mede outro.

Um experimento sem nenhuma mudança é a versão como publicada, e mantém o
fallback.

## Repetições e custo

Repetições servem para dar certeza, não para tirar média. Um modelo pode
responder o mesmo caso de jeitos diferentes a cada vez, e um caso que quebrou
em **qualquer** repetição está quebrado.

Cada repetição é um run, reservado e cobrado. Antes de começar, o formulário
mostra casos × repetições = runs, e quanto isso pode custar no máximo pelo teto
por run da versão. Cinco repetições de quarenta casos são duzentos runs.

## Leia o resultado

O resultado mostra, nesta ordem, o que rodou e depois o que encontrou:

1. versão do dataset;
2. versão do agente;
3. configuração, ou **Como publicado**;
4. casos rodados, dos casos da versão;
5. casos apagados, que não puderam rodar;
6. repetições;
7. quantos seguraram, quantos quebraram, e o custo.

Cada caso aparece como **Segurou**, **Quebrou**, **Apagado** ou **Não rodou**,
e em quantas repetições quebrou, das que rodaram: `1 de 5`. Um de cinco é um
modelo que às vezes erra; cinco de cinco é um modelo que erra. Quando rodaram
menos repetições do que o pedido, a linha diz isso.

O relatório pode ser lido enquanto o experimento ainda roda; os casos que
terminaram já são linhas completas.

## Compare dois experimentos

Na lista, marque dois experimentos e escolha **Comparar os dois**. Eles são
sempre comparados do mais antigo para o mais novo, qualquer que seja a ordem em
que você marcou.

Na **mesma versão do dataset**, os dois fizeram as mesmas perguntas e todo
caso é comparado. Em **versões diferentes do dataset**, não: a comparação lista
só os casos que as duas versões têm, avisa isso antes de tudo e conta quantos
cada lado tinha que o outro não tinha.

O custo varia **por run**, então um experimento com cinco repetições não
aparece como cinco vezes mais caro que um com uma repetição pelo mesmo preço.
Um caso que um lado nunca rodou está quebrado nesse lado, e não mexe no custo.

## Pela API

| Requisição | O que faz |
|---|---|
| `POST /agents/{agentId}/experiments` | Inicia um experimento. Aceita `version`, `datasetVersion`, `config` e `repetitions`, todos opcionais. |
| `GET /agents/{agentId}/experiments` | Os 20 experimentos abertos mais recentemente. |
| `GET /agents/{agentId}/experiments/{experimentId}` | O relatório de um experimento. |
| `GET /agents/{agentId}/experiment-comparison?from=&to=` | Dois experimentos, caso a caso. |

A `Idempotency-Key` dá nome ao experimento. Pedir de novo com a mesma chave
chega aos runs que ele já abriu, mesmo que o dataset ou o agente já tenha uma
versão mais nova, e abre os runs que uma primeira tentativa interrompida deixou
sem abrir. Um campo que o retry não manda é o da primeira tentativa. Um retry
que pede outra coisa é recusado com 409 e não abre nada. Use uma chave nova
para um experimento novo.

Um experimento fica onde os runs dele foram gravados. Se um agente for
republicado em outra área, os experimentos antigos ficam onde estavam: quem
pode ler a nova área recebe 404 para eles, como para um experimento que não
existe.

## Erros comuns

### "O experimento ganhou, então o agente agora usa aquele modelo"

Não usa. Publique uma nova versão com o modelo escolhido e rode a bateria dela.

### "Só mudei o esforço, mas o run não caiu para o fallback"

Essa é a regra: qualquer mudança roda sem o fallback declarado, para o
resultado medir o modelo que você escolheu. Deixe todos os campos como
publicado para manter o fallback.

### "A comparação deixou casos de fora"

Os dois experimentos rodaram versões diferentes do dataset. Só os casos que as
duas têm são comparados, e a tela diz quantos cada lado tinha sozinho.
