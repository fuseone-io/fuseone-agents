---
title: Lista de bloqueio Cloudflare
summary: Deixe um agente bloquear um atacante na borda da Cloudflare, dentro de guardas que vivem em código e bloqueios que expiram por desenho.
section: integrations
tags: cloudflare, firewall, block list, security, connector
order: 19
---

# Lista de bloqueio Cloudflare

O conector dá ao agente um poder estreito: adicionar o endereço de um
atacante a uma lista de IPs da Cloudflare que uma regra de firewall já
bloqueia na borda. Ele escreve essa lista e nada mais — nunca regras, zonas
ou DNS — e as guardas que o limitam vivem no código da plataforma, não em
prompt nenhum.

## O desenho em volta

O conector é uma peça de um desenho em três partes; as outras duas vivem na
infraestrutura do próprio operador:

1. **Uma regra de firewall** na conta Cloudflare com a expressão
   `ip.src in $sua_lista → Block`. A regra é gerenciada onde o operador
   gerencia a Cloudflare (Terraform, dashboard); o conector nunca a toca.
2. **Este conector**, que adiciona entradas à lista que a regra lê.
3. **Um job de expiração** operado por você (um cron em qualquer lugar), que
   remove entradas cujo comentário diga que já são velhas o bastante.
   Bloqueios feitos aqui são temporários por desenho: um engano se desfaz
   sozinho, e um atacante real que voltar é simplesmente bloqueado de novo.

## O contrato do comentário

Toda entrada escrita pelo conector carrega o comentário

```
fuseone:auto:<timestamp RFC3339> <motivo>
```

O timestamp é o que o job de expiração confia; o motivo são as palavras do
agente, achatadas em uma linha e limitadas. Entrada sem o prefixo
`fuseone:auto:` foi adicionada por uma pessoa, e nem este conector nem o job
de expiração devem tocá-la.

## O que o block_ip recusa, em código

- Qualquer coisa que não seja um endereço IP literal: CIDR, hostname, lixo.
- Endereços privados, loopback, link-local, multicast e não-especificados —
  incondicionalmente; nenhuma configuração reabre isso.
- As **faixas protegidas** da instância: CIDRs que o operador lista para a
  saída da própria instalação e para faixas de anonimizador compartilhadas
  por muitos clientes reais.
- O endereço quando as entradas automáticas do dia já atingiram o **teto
  diário** da instância — contado da própria lista.

Endereço que já está na lista retorna sucesso sem uma segunda escrita, então
a lista nunca cresce duplicatas. Toda recusa é uma falha limpa de ferramenta,
com código registrado no run.

O `unblock_ip` é a direção contrária, sob a guarda mais estrita de todas:
remove **somente entradas que o próprio conector escreveu** — o comentário
`fuseone:auto:`. Entrada de pessoa, ou faixa mais larga de pessoa cobrindo o
endereço, é decisão de pessoa: a chamada é recusada e o endereço continua
bloqueado. Endereço fora da lista já está no estado desejado e retorna
sucesso dizendo isso.

## Configurando uma instância

Integrações → Conectores → Nova lista de bloqueio Cloudflare. Account ID e
List ID nomeiam a única lista; o API token é selado ao salvar e nunca
retorna — o card só diz se há um guardado. O token precisa de exatamente uma
permissão na Cloudflare: editar filter lists da conta. Não dê mais nada, e a
instrução "escreva só a lista" deixa de ser promessa.

## Governança

`cloudflare.<instância>.block_ip` é efeito de escrita. Por padrão nenhum card
de aprovação é levantado — o caso de uso é a madrugada em que nenhum humano
está acordado — mas uma policy pode exigir um, e uma policy com alcance
"agentes" entrega a ferramenta a exatamente um agente. O ledger do run
registra cada bloqueio e cada recusa de qualquer forma.
