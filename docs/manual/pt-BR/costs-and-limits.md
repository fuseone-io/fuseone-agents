---
title: Custos e limites
summary: Como preço, moeda, teto por run e teto por área se relacionam sem misturar referência de mercado com contabilidade.
section: cost
tags: custo, preço, moeda, teto, orçamento, tokens, provider
order: 6
---

## O custo é calculado por tarifa configurada

A plataforma conta tokens em cada run. Para transformar tokens em dinheiro, a
instalação precisa ter uma tarifa configurada para o par provedor/modelo usado.

Preço de mercado exibido na tela é referência. Ele ajuda o operador a preencher
o formulário, mas não conta em tetos até ser salvo como tarifa da instalação.
Isso evita somar dólares de mercado dentro de uma instalação configurada em
BRL, EUR ou outra moeda.

## Por que uma run pode mostrar custo zero

Uma run com tokens e custo zero quase sempre significa uma destas coisas:

| Sintoma | Leitura |
|---|---|
| Tokens aparecem, custo R$0.00 | Não há tarifa configurada para aquele provedor/modelo |
| Teto bloqueou, mas custo é zero | O teto atingido pode ser de passos, chamadas ou duração, não dinheiro |
| Tarifa foi criada e run antiga segue zero | Histórico não é reprecificado |
| Tarifa foi criada e run nova segue zero | Worker ainda não carregou a revisão, ou provedor/modelo não casam exatamente |

O nome do modelo precisa casar com o que a run usa. `claude-opus-5` e
`claude-opus-5-20260801` são preços diferentes se o provedor registrar nomes
diferentes.

## Moeda

A moeda da instalação diz como renderizar valores já gravados e tarifas
configuradas. Trocar a moeda não converte histórico. Ela muda como os números
são lidos.

Não use a troca de moeda como conversão financeira. Se a instalação mudou de
BRL para USD, revise tetos e tarifas explicitamente.

## Tetos

Uma run pode parar por quatro famílias de limite:

- dinheiro;
- tokens ou custo estimado;
- número de ferramentas chamadas;
- passos ou duração.

Quando o Gate bloqueia, leia a frase do evento. Ela diz qual teto foi atingido:
dinheiro, chamadas, passos ou outro limite. Configurar preço resolve só o teto
financeiro; não aumenta limite de passos.

## Reserva e reconciliação

Antes de gastar, a plataforma reserva o custo estimado. Depois da chamada, ela
reconcilia com o custo real e libera o que sobrou.

Isso é por tempo, não por contabilidade bonita: se a plataforma só contasse
depois, vinte workers poderiam abrir chamadas ao mesmo tempo antes de qualquer
um registrar o custo.

## Cache reads e cache writes

Alguns provedores cobram entrada cacheada por preço menor. A tela separa input,
output, cache read e cache write porque eles não custam igual.

Mesmo quando cache read é barato, ele ainda é consumo. Otimização boa é aquela
que aparece na contabilidade, não a que desaparece dela.

Para Anthropic, o FuseOne marca como cacheáveis o texto de sistema estável e o
prefixo anterior da trilha reconstruída. Orientação da etapa, orientação de
memória e nota de orçamento restante ficam depois desse prefixo, porque podem
mudar a cada turno. Tokens de cache read e cache write continuam visíveis na
execução e nas métricas de baixa cardinalidade do worker.

## Descobrir o que deixou o prompt grande

A trilha da run mostra o conteúdo do prompt em cada proposta do modelo:
instruções, entrada, notas da plataforma e resultados de ferramenta.

Esses números são **bytes de conteúdo**, medidos pela plataforma enquanto monta
o pedido ao modelo. Eles não são tokens e não são custo. Tokens e dinheiro
continuam vindo do uso reportado pelo provedor e da tarifa configurada na
instalação.

Use essa linha para escolher a próxima otimização. Se resultados de ferramenta
dominarem, reduza ou resuma a saída daquela ferramenta. Se instruções
dominarem, reescreva os blocos do agente. Se entrada dominar, envie menos
contexto para a run.

Entradas grandes vindas de canal também são compactadas antes de chegar ao
modelo. A ask completa continua no content store e na trilha; o modelo recebe
um JSON válido com campos longos encurtados e uma nota separada da plataforma
com tamanho guardado e digest. Isso aparece principalmente quando um alerta ou
thread do Slack traz um payload grande antes de o agente chamar qualquer
ferramenta.

Resultados grandes de Grafana Loki e Prometheus, material de revisão do GitHub
e resultados de fetch, list e search do Outline são compactados antes dos
turnos seguintes do modelo. Tamanho e digest do resultado completo continuam na
trilha; o content store mantém sua cópia limitada pelo teto da instalação. O
modelo recebe uma visão compacta com começo, fim, tamanho original e digest.

Também existe um orçamento total para resultados no transcript. Evidência
recente é preservada; resultados antigos viram recibos explícitos com
ferramenta, tamanho do resultado original, digest e bytes omitidos. Recibo nunca
significa que o conteúdo não existia e não apaga a cópia armazenada. Ele orienta
o modelo a fazer outra chamada somente quando conseguir restringir
materialmente a consulta.

Métricas do worker expõem bytes agregados de resultados no prompt como `sent`
e `elided`, tokens de cache como `read` e `write`, duplicatas canônicas puladas
e investigações estacionadas. Elas deliberadamente não usam execução, agente,
ferramenta, pessoa ou consulta como label; a atribuição por ferramenta fica na
trilha de cada execução.

## Checklist de configuração

1. Configure a moeda da instalação.
2. Confira o provedor/modelo que aparece na run.
3. Abra Cost and limits e crie uma tarifa para esse par exato.
4. Use o preço de mercado como referência, não como valor automático.
5. Rode uma run nova.
6. Se o custo continuar zero, confira se o worker carregou a revisão e se o
   nome do modelo bate exatamente.
