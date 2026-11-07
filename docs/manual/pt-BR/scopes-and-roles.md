---
title: Empresas, áreas e papéis
summary: Onde um agente vive, quem alcança o quê, e por que administrar identidade exige o escopo da instalação.
section: governance
tags: escopo, empresa, área, papel, permissão, administrador, grant
order: 9
---

## Duas dimensões: onde e quem

**Empresa e área** dizem *onde* uma coisa vive. Um agente é publicado numa dupla empresa/área, e é ali que ele roda, gasta e é auditado.

**Papel** diz *o que* uma pessoa pode fazer. E papel nunca vale sozinho — ele é sempre concedido **dentro de um escopo**.

Um mesmo alguém pode ser autor em `acme/plataforma` e apenas aprovador em `acme/financeiro`. Isso não é exceção; é o desenho.

## Área é escopada por empresa

`plataforma` dentro de `acme` e `plataforma` dentro de `default` são **áreas diferentes**, apesar do nome igual.

Isso importa na hora de publicar: você escolhe a dupla, não só a área. Um agente publicado na dupla errada roda com as permissões de outro lugar.

## Os papéis

| Papel | Para quê |
|---|---|
| **Autor** | Escreve, ensaia e publica agentes |
| **Aprovador** | Decide as chamadas que pararam |
| **Curador** | Classifica ferramentas e cuida da superfície |
| **Administrador** | Tudo, no escopo da instalação |

Não há herança e não há curinga: **ler uma linha da tabela diz tudo que aquele papel pode fazer**. Um autor não classifica ferramenta; um aprovador não publica agente.

## O escopo da instalação

Existe um escopo acima de todos: empresa `*` com área vazia. É o que alcança todas as empresas e áreas, inclusive as criadas amanhã.

**Administrador concedido ali** é o administrador da instalação — bootstrap, identidade, integrações, marca, tetos globais.

**Administrador concedido numa empresa ou área comum** continua escopado: ele pode tudo, mas só ali.

## Por que identidade exige a instalação

Criar pessoas, mudar grants e trocar senha local só funcionam com `identity:write` **no escopo da instalação**.

A razão é concreta: quem administra identidade pode conceder administrador a qualquer um, inclusive a si mesmo, em qualquer lugar. Se essa permissão valesse dentro de uma área, quem administrasse aquela área poderia cunhar administradores da instalação inteira — e teria feito isso a partir de um escopo que ninguém considera privilegiado.

## Grant vindo do provedor de identidade

Quando o login é por provedor externo, um grant pode vir de um **grupo** que ele afirma. Esses são **rederivados a cada login**.

Consequência prática: **revogar esse grant na tela dura até a pessoa entrar de novo.** O que precisa mudar é o grupo, no provedor. A tela de Pessoas marca a origem de cada grant — `local`, `provider` ou `mixed` — exatamente para essa distinção não ser descoberta na segunda-feira.

## Contas de serviço

Um pipeline, um script ou uma execução de Terraform agem por uma conta de serviço, em **Administração > Contas de serviço**. Ali existem três atos, e eles significam coisas diferentes:

- **Revogar um token** encerra aquela credencial em definitivo, na próxima requisição.
- **Desabilitar a conta** para todos os tokens de uma vez. Dá para desfazer: habilitar traz de volta os tokens não revogados e não expirados, e o diálogo diz quantos são.
- **Alterar o que ela pode fazer** muda os papéis e não afeta os tokens.

Um token é emitido com validade obrigatória, e o segredo aparece uma única vez. Só o hash fica guardado, então copie antes de fechar o diálogo. Para rotacionar, emita um substituto com o mesmo nome, passe a máquina para ele e depois revogue o antigo: os dois funcionam nesse intervalo, e o id os distingue.

Seja qual for o papel, uma conta de serviço nunca pode aprovar uma ação, conceder aprovação permanente nem mudar quem tem qual acesso. Ela também não pode ter senha nem sessão, então revogar os tokens encerra o acesso dela. Cada um desses atos fica na trilha administrativa, sem o segredo.

## Casos de uso

### Um time cuida do próprio agente

Autor em `empresa/área-do-time`. Ele escreve, ensaia e publica ali, e não alcança nada de outra área.

### Alguém aprova sem poder editar

Aprovador na área. Vê as execuções e decide o que parou, e não consegue mudar a instrução para contornar a própria recusa.

### Quem monta a plataforma

Administrador no escopo da instalação — empresa `*`, área vazia. É um grant, não quatro papéis montados à mão.

### Um time que também classifica as próprias ferramentas

Autor **e** curador na mesma área. Classificar é julgamento sobre o que a ferramenta faz ao mundo, e é razoável ficar com quem conhece o domínio — desde que fique registrado que ficou.

O que cada estágio permite está em [Rascunho, copiloto e autônomo](autonomy-stages.md).
