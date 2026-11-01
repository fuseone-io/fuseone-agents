---
title: Monitorando o despacho de eventos
summary: Quanto o despacho de eventos está atrasado, quando uma transação aberta no banco o está segurando, e as métricas e o sinal de saúde que mostram isso.
section: operations
tags: monitoramento, prometheus, métricas, eventos, despacho, xmin, transação, saúde, runtime
order: 24
---

## O que pode atrasar um evento

Quando uma execução termina e o agente dela publica um evento, o despachante
de eventos do worker abre as execuções dos agentes que escutam esse evento. O
despachante faz uma varredura a cada quinze segundos e lê as execuções
finalizadas para frente a partir de um único cursor para toda a instalação.

Ele lê apenas execuções que terminaram antes de a **transação aberta mais
antiga do banco** começar. É isso que garante que nenhuma execução seja
pulada: uma transação ainda aberta poderia confirmar uma execução finalizada
com uma posição anterior, e o cursor não pode ter passado por ela. O preço é
que uma transação longa em qualquer lugar do banco — um relatório, uma sessão
de `psql` esquecida aberta, uma migração travada — atrasa todo despacho de
eventos enquanto estiver aberta. O despacho atrasa, nunca pula: quando a
transação termina, as execuções em espera são despachadas na próxima
varredura.

## Na página Saúde do runtime

Quem tem acesso à instalação inteira vê um painel **Despacho de eventos**:

| Campo | Significado |
|---|---|
| Aguardando a próxima varredura | Execuções finalizadas depois do cursor que o despachante já pode ler. Normalmente zero ou poucas, resolvidas em até quinze segundos. |
| Retidas por uma transação aberta | Execuções finalizadas que foram confirmadas depois que a transação aberta mais antiga começou. Varrer mais vezes não as alcança; encerrar essa transação, sim. |
| A execução mais antiga na fila terminou há … | O atraso: quanto tempo a execução finalizada mais antiga depois do cursor está esperando. |
| Idade da transação aberta | Há quanto tempo a transação aberta mais antiga está aberta, exibida quando há execuções retidas. |
| Saudável / Degradado | Fica degradado quando o atraso passa de cinco minutos. |

Um despachante degradado também aparece em **Precisa de atenção**. Degradado
não é falha: nada se perde e a plataforma continua funcionando.

Quem tem acesso a uma única empresa ou área não vê este painel. O cursor
pertence à instalação e a contagem cobre as execuções de todos os escopos.

## Métricas do Prometheus

O endpoint de métricas do worker (`--metrics-addr`) expõe, depois de cada
varredura:

| Métrica | Tipo | Significado |
|---|---|---|
| `fuseone_event_dispatch_backlog_runs{state="ready"}` | gauge | Execuções aguardando a próxima varredura. |
| `fuseone_event_dispatch_backlog_runs{state="held_by_xmin"}` | gauge | Execuções retidas pela transação aberta mais antiga. |
| `fuseone_event_dispatch_lag_seconds` | gauge | Quanto tempo a execução mais antiga na fila está esperando. |
| `fuseone_event_dispatch_held_by_xmin` | gauge | 1 enquanto execuções confirmadas esperam uma transação aberta. |
| `fuseone_event_dispatch_oldest_xact_age_seconds` | gauge | Idade da transação aberta mais antiga que detém um id. Omitida quando desconhecida. |
| `fuseone_event_dispatch_oldest_xact_age_known` | gauge | 0 quando há uma transação aberta e não é possível ver quando ela começou. |
| `fuseone_event_dispatch_degraded` | gauge | 1 quando o atraso passa de cinco minutos. |
| `fuseone_event_dispatch_observed_timestamp_seconds` | gauge | Quando o worker leu a fila pela última vez. |
| `fuseone_event_dispatch_observe_failures_total` | counter | Leituras da fila que falharam. |

O único rótulo é `state`, com dois valores. Nenhum id de execução, agente ou
transação vira rótulo. Os gauges só aparecem depois da primeira leitura bem
sucedida, então um worker que nunca olhou não consegue dizer que o despacho
está em dia.

Um ponto de partida para alertas:

```
fuseone_event_dispatch_degraded == 1                     for 5m
time() - fuseone_event_dispatch_observed_timestamp_seconds > 120
```

## Vendo a idade da transação

A idade da transação aberta vem de `pg_stat_activity`. O PostgreSQL mostra a
qualquer papel que outra sessão detém uma transação, mas só mostra **quando
ela começou** a um papel que seja superusuário, membro do papel da sessão ou
membro de `pg_read_all_stats`. Um papel de aplicação comum vê que a transação
está aberta, mas não a idade: o painel diz isso e a métrica
`fuseone_event_dispatch_oldest_xact_age_known` fica em 0.

Para ver a idade, conceda o papel de monitoramento ao papel com que a
plataforma se conecta:

```sql
grant pg_read_all_stats to <papel da plataforma>;
```

Nada mais depende dessa concessão. Sem ela — e mesmo que `pg_stat_activity`
tenha sido revogado — a contagem da fila, o atraso e o sinal de saúde
continuam funcionando.

## Quando o despacho está retido

1. Veja `fuseone_event_dispatch_held_by_xmin`. Em 0 com o atraso crescendo, é
   o próprio despachante que não está varrendo: procure
   `event dispatch failed` nos logs do worker.
2. Em 1, encontre a transação aberta mais antiga:

   ```sql
   select pid, usename, application_name, state, xact_start, left(query, 80)
     from pg_stat_activity
    where backend_xid is not null
    order by age(backend_xid) desc
    limit 5;
   ```

   Ela pode estar em qualquer banco do mesmo cluster PostgreSQL.
3. Encerrar essa transação libera o despacho. As execuções retidas são
   despachadas na próxima varredura, cada uma uma vez.
