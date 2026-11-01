---
title: Fábrica de software
summary: Rode papéis de agentes que fazem a triagem de issues e revisam pull requests num repositório do GitHub, cada um com o seu GitHub App e toda escrita aprovada por uma pessoa.
section: integrations
tags: fábrica, github, github app, papéis, triagem, revisão, qa, copiloto
order: 22
---

# Fábrica de software

Uma fábrica de software é um conjunto de agentes, cada um num **papel**, que
levam as issues de um repositório da triagem até um pull request pronto para
merge, guiados por um board do GitHub Projects. A plataforma roda isso só com
configuração: os papéis são agentes comuns, o servidor MCP oficial do GitHub é
o único servidor de ferramentas deles, e o Gate governa cada chamada que fazem.

Esta página é o caminho de quem opera. O raciocínio por trás está na nota de
desenho
[NT-013](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-013-software-factory-pilot.md),
e as instruções dos papéis ficam, revisadas como código, em
[`factory/`](https://github.com/fuseone-io/fuseone-agents/tree/main/factory).

## O que é a primeira versão

- **Três papéis.** A Triagem transforma uma issue nova num card preparado; o
  Revisor revisa um pull request; o QA confere o pull request contra o CI e os
  critérios de aceite. Cada papel é um agente criado no console ou no Studio.
- **Um GitHub App por papel.** Cada papel só alcança o GitHub pelo seu próprio
  registro do servidor do GitHub, autenticado como o seu próprio App. As
  permissões do App são o primeiro limite, e o nome dele é o que assina os
  comentários e revisões do papel.
- **Todos os papéis em copiloto**, com aprovações permanentes proibidas. Toda
  escrita que um papel propõe — comentário, rótulo, revisão, card movido —
  espera uma pessoa. Veja [Rascunho, copiloto e autônomo](autonomy-stages.md).
- **Uma pessoa faz o merge.** Nenhum papel faz merge.
- **O board guia o trabalho**, com cinco colunas: Triagem, Preparado, Doing,
  Travado e Done. Dentro de Doing, rótulos marcam em que ponto o pull request
  está: `factory:needs-review`, `factory:changes-requested`,
  `factory:needs-qa` e `factory:ready-to-merge`. Só uma pessoa tira um card de
  Travado.
- **Um card por run.** Um papel acorda no seu horário, escolhe um card, faz o
  claim dele com `$fuseone.work_item.claim` e trabalha só nesse card. O claim
  fica registrado na trilha do run, e nenhum outro run do mesmo papel pode
  pegar um card que já está reservado. Antes do claim, a Triagem vê só os
  títulos dos cards, porque a listagem do board sempre traz os títulos. Um
  título é texto não confiável: o papel usa o título só para diferenciar os
  cards e lê o corpo e os comentários da issue depois do claim. O que contém o
  título é o copiloto, com toda escrita aprovada por uma pessoa. Uma
  ferramenta que escolhe e faz o claim do card de uma vez, escondendo até o
  título, ficou para depois.

O repositório é público, então o texto de issues e pull requests não é
confiável: um run que leu esse texto nunca pode executar uma ação destrutiva,
e as escritas dele precisam de uma pessoa.

## 1. Crie os GitHub Apps

Crie um GitHub App por papel, instale cada um só no repositório em que a
fábrica trabalha e dê a ele só o que as ferramentas do papel precisam:

| Papel | Permissões no repositório | Permissões na organização |
|---|---|---|
| Triagem | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Revisor | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |

O veredito do QA é uma revisão de pull request, então o QA precisa de Pull
requests: write, assim como o Revisor. Gere uma chave privada para cada App e
anote o ID do App e o ID da instalação.

## 2. Registre o servidor do GitHub uma vez por papel

Na administração, registre o GitHub MCP hospedado
(`https://api.githubcopilot.com/mcp/`) três vezes, uma por papel, com o id
`github-<papel>`: `github-triage`, `github-reviewer`, `github-qa`. Escolha a
credencial **GitHub App** e preencha o App daquele papel (veja
[MCPs e credenciais](mcp-and-credentials.md), seção "GitHub App").

Os toolsets padrão do servidor hospedado não incluem `projects`, então
preencha o campo **Toolsets** (enviado como `X-MCP-Toolsets`) em todos os
registros:

| Papel | Toolsets |
|---|---|
| Triagem | `issues,projects` |
| Revisor | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |

O servidor hospedado ignora em silêncio um nome de toolset que não conhece,
então um erro de digitação só aparece depois, como uma publicação recusada por
falta de ferramentas.

## 3. Classifique as ferramentas

Com cada servidor respondendo, classifique todas as ferramentas que os papéis
declaram (veja [O que a plataforma para antes de acontecer](what-the-gate-stops.md)).

**Toda ferramenta de leitura é não confiável (untrusted)** — não só as que
leem o corpo de uma issue. Uma listagem devolve texto escrito por pessoas: o
`projects_list` devolve os títulos das issues e pull requests do board mesmo
quando não se pede nenhum campo (#220).

| Classifique como | Ferramentas |
|---|---|
| read, **untrusted** | `projects_list`, `projects_get`, `search_pull_requests`, `search_issues`, `list_issues`, `issue_read`, `pull_request_read`, `get_file_contents`, `search_code`, `get_commit` |
| write | `add_issue_comment`, `issue_write`, `pull_request_review_write`, `projects_write` |

Classifique cada ferramenta no servidor de cada papel:
`github-triage.projects_list` e `github-reviewer.projects_list` são entradas
separadas.

O `projects_write` move cards e também pode remover um item. Classifique como
**write**, não como destructive: um card removido pode ser adicionado de novo,
e uma pessoa aprova toda escrita. Classificado como destrutivo, ele seria
bloqueado de vez, e todo papel que lê texto de issue e move cards seria
recusado na publicação.

## 4. Crie a área da fábrica

Crie uma área para a fábrica, para que custo, orçamento e concessões dela
fiquem separados de todos os outros agentes, e dê um orçamento a ela (veja
[Custos e limites](costs-and-limits.md) e
[Empresas, áreas e papéis](scopes-and-roles.md)). As referências usam o id de área
`factory`.

Cada referência também traz um teto por run, para que um papel em loop pare
em vez de gastar:

| Papel | Horário | Teto por run | Chamadas de ferramenta | Passos |
|---|---|---|---|---|
| Triagem | toda hora, aos :00 | 500.000 micros | 30 | 60 |
| Revisor | toda hora, aos :15 | 2.000.000 micros | 80 | 160 |
| QA | toda hora, aos :45 | 1.000.000 micros | 50 | 100 |

Micros são milionésimos da moeda configurada na instalação. Trate esses
valores como uma primeira calibração.

## 5. Crie os papéis

Para cada papel, crie o agente no Studio a partir da referência: os campos do
frontmatter, e o corpo como instruções.

- [Triagem](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/triage.agent.md)
- [Revisor](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/reviewer.agent.md)
- [QA](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/qa.agent.md)

Depois publique — um agente novo é um rascunho pausado —, ponha o estágio em
**copiloto** e inicie. O estágio não está no arquivo, então ele sempre é
definido no console.

O console recusa publicar uma ferramenta que o servidor registrado não
oferece, então um nome de ferramenta errado, ou um toolset faltando, aparece
aqui, e não na hora do run.

O que os papéis seguem está no mesmo diretório:
[RULEBOOK](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/RULEBOOK.md)
(as regras que todo papel segue),
[REVIEW](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/REVIEW.md)
(o formato da revisão) e
[LANES](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/LANES.md)
(o que significa pronto em cada coluna e rótulo). O
[README das referências](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/README.md)
explica cada linha de uma referência.

## Mudando um papel

Uma mudança num papel é feita primeiro na referência, revisada por pull
request, e depois copiada para o console. Quando o console e a referência
discordam, foi o console que se desviou.

Depois de mudar os toolsets de um registro, confira se o servidor aparece como
alcançável antes de confiar numa publicação. A próxima descoberta substitui as
ferramentas dele: uma ferramenta que os toolsets novos não oferecem sai do
catálogo, e a checagem da publicação recusa um papel que ainda a cita. Um
servidor que não pode ser alcançado mantém as ferramentas da última descoberta
bem-sucedida, então, até ser alcançado de novo, o catálogo ainda mostra o
conjunto antigo.
