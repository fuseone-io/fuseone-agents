---
title: Fábrica de software
summary: Rode papéis de agentes que fazem a triagem de issues, planejam, fazem mudanças pequenas e revisam pull requests num repositório do GitHub, cada um com o seu GitHub App e toda escrita no GitHub aprovada por uma pessoa.
section: integrations
tags: fábrica, github, github app, papéis, triagem, arquiteto, plano, desenvolvedor, revisão, qa, copiloto
order: 22
---

# Fábrica de software

Uma fábrica de software é um conjunto de agentes, cada um num **papel**, que
levam as issues de um repositório da triagem até um pull request pronto para
merge, guiados por um board do GitHub Projects. A plataforma roda isso só com
configuração: os papéis são agentes comuns, as ferramentas deles vêm do
servidor MCP oficial do GitHub e do servidor MCP da própria fábrica, que move
os cards do board e registra o veredito do QA, e o Gate governa cada chamada
que fazem.

Esta página é o caminho de quem opera. O raciocínio por trás está na nota de
desenho
[NT-013](https://github.com/fuseone-io/fuseone-agents/blob/main/docs/NT-013-software-factory-pilot.md),
e as instruções dos papéis ficam, revisadas como código, em
[`factory/`](https://github.com/fuseone-io/fuseone-agents/tree/main/factory).

## O que é a primeira versão

- **Cinco papéis.** A Triagem transforma uma issue nova num card preparado,
  ou passa o card para o Arquiteto com `factory:needs-design` quando ele
  precisa de um plano; o Arquiteto confere a mudança no código e escreve o
  plano e os critérios de aceite na issue; o Desenvolvedor faz uma mudança pequena e abre o pull request dela; o
  Revisor revisa um pull request; o QA confere o pull request contra o CI e os
  critérios de aceite. Cada papel é um agente criado no console ou no Studio.
- **Um GitHub App por papel.** Cada papel só alcança o GitHub pelos seus
  próprios registros, o do servidor do GitHub e o do servidor da fábrica, os
  dois autenticados como o seu próprio App. As permissões do App são o
  primeiro limite, e o nome dele é o que assina os comentários e revisões do
  papel e o status `factory/qa` do QA.
- **Ferramentas de board estreitas.** Os cards andam com o `move_card` do
  servidor da fábrica, só pelas transições do próprio papel (a Triagem e o
  Arquiteto de Triagem para Preparado ou Travado; o Desenvolvedor, o Revisor e
  o QA de Doing para Travado), e nada sai de Travado. A Triagem e o Arquiteto
  preenchem Work type e Size com o `set_card_fields`, só em Triagem. Nenhum
  papel tem o `projects_write` do servidor do GitHub.
- **Todos os papéis em copiloto**, com aprovações permanentes proibidas. Toda
  escrita no GitHub que um papel propõe — comentário, rótulo, revisão, card
  movido — espera uma pessoa. O claim do card, descrito abaixo, e o vínculo
  que o Desenvolvedor faz do pull request que acabou de abrir são as escritas
  que não esperam, e nenhuma das duas escreve no GitHub. Veja
  [Rascunho, copiloto e autônomo](autonomy-stages.md).
- **Uma pessoa faz o merge.** Nenhum papel faz merge.
- **O board guia o trabalho**, com cinco colunas: Triagem, Preparado, Doing,
  Travado e Done. Dentro de Doing, rótulos marcam em que ponto o pull request
  está: `factory:needs-review`, `factory:changes-requested`,
  `factory:needs-qa` e `factory:ready-to-merge`. Um card que o
  Desenvolvedor não consegue fazer sem um build recebe `factory:needs-runtime`
  na issue. Só uma pessoa tira um card de Travado.
- **Um card por run.** Um papel acorda no seu horário, escolhe um card, faz o
  claim dele com `$fuseone.work_item.claim` e trabalha só nesse card. O claim
  fica registrado na trilha do run, e nenhum outro run do mesmo papel pode
  pegar um card que já está reservado. Antes do claim, a Triagem é instruída
  a listar a coluna só com o `projects_list`, método `list_project_items`, e
  assim vê os títulos dos cards, que a listagem do board sempre traz. As
  mesmas ferramentas de Projects também podem devolver os status updates do
  board, que são texto livre, e nada na plataforma impede o modelo de pedir
  esses dados, porque o Gate classifica a ferramenta, não o método. O piloto
  aceita isso dentro dos limites do claim descritos abaixo; impedir de fato
  exige a ferramenta específica que ficou para depois. Um
  título é texto não confiável: o papel é instruído a usar o título só para
  diferenciar os cards e a escolher por idade e rótulo, e lê o corpo e os
  comentários da issue depois do claim. Mesmo assim, um título pode
  influenciar qual card o modelo escolhe, e o claim não precisa de aprovação,
  nem depois de texto não confiável: é uma operação de coordenação, para que
  dois runs nunca esperem uma pessoa para reservar um card. Um claim no card
  errado só reserva esse card para o papel enquanto o run está aberto, libera
  o card quando o run termina e lança o custo do run nesse card. Ele não
  escreve nada no GitHub, e toda escrita no GitHub depois dele espera uma
  pessoa. Uma ferramenta que escolhe e faz o claim do card de uma vez,
  escondendo até o título, ficou para depois. O Arquiteto lista a coluna
  Triagem, e o Desenvolvedor a coluna Doing, do mesmo jeito, e os dois
  recebem a mesma instrução.
- **O histórico de um card.** A linha **Item** do claim, na trilha de um run,
  leva a todos os runs que fizeram o claim do card, com as aprovações e o
  custo de cada papel (veja [Ler uma execução](reading-a-run.md)). A issue e
  os pull requests vinculados a ela aparecem juntos: logo depois de abrir um
  pull request, o Desenvolvedor o vincula à issue do seu claim com
  `$fuseone.work_item.link`, e a partir daí os runs do Revisor e do QA nesse
  pull request entram no histórico da issue, marcados como **Pull request**.
  Abrir o histórico de um pull request vinculado mostra o histórico da issue
  e avisa isso. O vínculo não espera ninguém quando o próprio
  `create_pull_request` do run citou aquele pull request — respondido pelo
  servidor hospedado do GitHub registrado com o GitHub App do papel — e espera
  uma pessoa nos outros casos. Mantenha o resultado do `create_pull_request`
  como **untrusted**, o padrão: o vínculo é provado pelo pull request que a
  plataforma lê da resposta, não pela resposta, que continua marcando o run
  como não confiável, e toda escrita seguinte continua esperando uma pessoa; um pull request pertence a um só card para sempre, e um
  vínculo que você não pode ler aparece para você como se não existisse.

O repositório é público, então o texto de issues e pull requests não é
confiável: um run que leu esse texto nunca pode executar uma ação destrutiva,
e as escritas dele no GitHub precisam de uma pessoa.

## 1. Crie os rótulos

O board encaminha pull requests e issues por rótulos, e o GitHub só aplica
rótulos que o repositório já tem. Um papel que tenta pôr um rótulo que nunca
foi criado falha nessa escrita, e o card fica onde estava, para ser escolhido
de novo no próximo run. Crie todos os rótulos da fábrica uma vez, a partir de
um clone do repositório em que a fábrica trabalha (ou acrescente
`--repo <dono>/<nome>`). Com `--force`, cada linha pode ser rodada de novo sem
problema:

```sh
gh label create factory:needs-design --force --color fbca04 --description "Triage asks the Architect for a plan"
gh label create factory:needs-review --force --color fbca04 --description "PR waits for the Reviewer"
gh label create factory:changes-requested --force --color fbca04 --description "Reviewer or QA asked for changes"
gh label create factory:needs-qa --force --color fbca04 --description "PR waits for QA"
gh label create factory:ready-to-merge --force --color fbca04 --description "Review and QA approved the head; a person merges"
gh label create factory:needs-runtime --force --color fbca04 --description "Needs a build or a test run before its PR; waits for Level 2"
```


## 2. Crie os GitHub Apps

Crie um GitHub App por papel, instale cada um só no repositório em que a
fábrica trabalha e dê a ele só o que as ferramentas do papel precisam:

| Papel | Permissões no repositório | Permissões na organização |
|---|---|---|
| Triagem | Issues: write, Pull requests: read, Metadata: read | Projects: write |
| Arquiteto | Contents: read, Issues: write, Metadata: read | Projects: write |
| Revisor | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: read, Metadata: read | Projects: write |
| QA | Pull requests: write, Issues: write, Contents: read, Checks: read, Commit statuses: write, Metadata: read | Projects: write |
| Desenvolvedor | Contents: write, Pull requests: write, Issues: write, Metadata: read | Projects: write |

O veredito do QA é o status de commit `factory/qa` no head que ele conferiu,
então o App dele precisa de Commit statuses: write; ele ainda comenta o
veredito e troca os rótulos do pull request, então continua com Pull
requests: write. O Arquiteto lê o código e só escreve
em issues e no board: dê ao App dele Contents: **read**, nunca write, para que
o GitHub recuse qualquer arquivo ou branch que tentem convencê-lo a escrever. Não dê a permissão Workflows ao App do
Desenvolvedor: sem ela, o GitHub recusa qualquer mudança que o Desenvolvedor
faça em `.github/workflows`. Gere uma chave privada para cada App e anote o
ID do App e o ID da instalação.

Contents: write alcança todos os branches, inclusive o `main`. O
Desenvolvedor é instruído a escrever só nos branches que ele mesmo cria em
`factory/`, e a pessoa que aprova cada escrita vê o branch que ela indica, mas
nenhum dos dois impede uma escrita. Por isso um ruleset é pré-requisito, não
recomendação:

Antes de iniciar o Desenvolvedor, o main precisa ter o ruleset mínimo: pull request obrigatório antes do merge, force push e exclusão bloqueados, e nenhum bypass para os GitHub Apps da fábrica.

Enquanto ele não existir, deixe o Desenvolvedor pausado. Em Settings → Rules →
Rulesets do repositório, crie um ruleset de branch com enforcement **Active**,
alvo `refs/heads/main`, marque **Require a pull request before merging** (zero
aprovações bastam: uma pessoa continua fazendo o merge), **Block force
pushes** e **Restrict deletions**, e não ponha ninguém na lista de bypass,
para que nenhum App da fábrica consiga pular as regras. Sem bypass vale
também para pessoas: a mudança de uma pessoa, inclusive o commit de
changelog de uma release, chega ao `main` por pull request, e a tag da
release vai no commit do merge (o ruleset vale para `refs/heads/main`, não
para tags). Ou pela API, trocando `<owner>/<name>`:

```sh
gh api --method POST repos/<owner>/<name>/rulesets --input - <<'EOF'
{
  "name": "main: pull request required",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "pull_request", "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false } },
    { "type": "non_fast_forward" },
    { "type": "deletion" }
  ],
  "bypass_actors": []
}
EOF
```

O resto do que o `main` deve exigir no futuro — status checks presos à sua
origem, o `factory/qa`, os revisores autorizados e as regras do Merger — ficou
para depois
([#207](https://github.com/fuseone-io/fuseone-agents/issues/207)); nada disso
é necessário na v1.

Antes de ativar o Merger, duas listas precisam estar aplicadas e ser idênticas:
os checks obrigatórios do ruleset e o `FACTORY_CHECKS` do Merger. As duas trazem
todos os jobs do workflow `check`, com os nomes que o GitHub mostra:
`go`, `postgres`, `console`, `chart`, `factory-mcp-image`, `artifact`. A parte do
ruleset é a #207; o `FACTORY_CHECKS` está descrito em `factory/mcp/README.md`.

## 3. Registre os servidores de ferramentas uma vez por papel

Na administração, registre o GitHub MCP hospedado
(`https://api.githubcopilot.com/mcp/`) cinco vezes, uma por papel, com o id
`github-<papel>`: `github-triage`, `github-architect`, `github-reviewer`,
`github-qa`, `github-developer`. Escolha a
credencial **GitHub App** e preencha o App daquele papel (veja
[MCPs e credenciais](mcp-and-credentials.md), seção "GitHub App").

Os toolsets padrão do servidor hospedado não incluem `projects`, então
preencha o campo **Toolsets** (enviado como `X-MCP-Toolsets`) em todos os
registros:

| Papel | Toolsets |
|---|---|
| Triagem | `issues,projects` |
| Arquiteto | `issues,projects,repos` |
| Revisor | `pull_requests,repos,issues,projects` |
| QA | `pull_requests,repos,issues,projects` |
| Desenvolvedor | `pull_requests,repos,issues,projects` |

O servidor hospedado ignora em silêncio um nome de toolset que não conhece,
então um erro de digitação só aparece depois, como uma publicação recusada por
falta de ferramentas.

Os toolsets oferecem mais do que um papel declara — o conjunto `projects`
também oferece o `projects_write`, que altera qualquer campo de qualquer card
e apaga cards, e que nenhum papel tem. Assim que um servidor responder, abra
a página **Ferramentas e superfície** dele e traga exatamente estas
ferramentas, nada além. Uma ferramenta deixada fora da superfície não existe
para os agentes: um papel que a cita é recusado na publicação, e uma chamada a
ela é recusada.

| Servidor | Ferramentas dele |
|---|---|
| `github-triage` | `projects_list,projects_get,issue_read,list_issues,search_issues,add_issue_comment,issue_write` |
| `github-architect` | `projects_list,projects_get,issue_read,search_issues,get_file_contents,search_code,list_commits,get_commit,add_issue_comment,issue_write` |
| `github-reviewer` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,search_code,get_commit,pull_request_review_write,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-qa` | `search_pull_requests,pull_request_read,issue_read,get_file_contents,issue_write,add_issue_comment,projects_list,projects_get` |
| `github-developer` | `search_pull_requests,projects_list,projects_get,issue_read,pull_request_read,get_file_contents,search_code,get_commit,create_branch,create_or_update_file,push_files,create_pull_request,add_issue_comment,issue_write` |

O servidor hospedado também pode ser estreitado do lado dele: o header
`X-MCP-Tools` com uma linha acima, e **sem** `X-MCP-Toolsets`, faz ele listar
só essas ferramentas (com os dois preenchidos, as ferramentas são somadas aos
conjuntos). O painel do App no console só preenche Toolsets, então esse header
é gravado pela API de administração; a superfície é obrigatória de qualquer
jeito.

### O servidor da própria fábrica

O servidor da fábrica roda como sidecar em cada pod do worker, em
`http://127.0.0.1:8091/`, sem credencial própria: o token de App de cada
requisição diz qual papel está chamando, e ele só serve as ferramentas desse
papel. Como implantá-lo, e o ConfigMap que ele lê, estão em
[`factory/mcp/README.md`](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/mcp/README.md#as-a-worker-sidecar).
Antes do primeiro worker subir com ele, confira que a imagem pode ser baixada
sem credencial:

```sh
gh api /orgs/fuseone-io/packages/container/fuseone-agents-factory-mcp --jq .visibility
```

A resposta precisa ser `public`; se não for, dê ao worker um pull secret do
GHCR.

Depois registre-o mais cinco vezes, com o id `factory-<papel>`:
`factory-triage`, `factory-architect`, `factory-reviewer`, `factory-qa`,
`factory-developer`. A URL é `http://127.0.0.1:8091/` (`http://` sem TLS só é
aceito ao lado de um GitHub App para loopback), a credencial é **GitHub App**,
e o App é **o mesmo** do `github-<papel>` daquele papel. Não preencha
toolsets. Cada registro então oferece as ferramentas do seu papel:

| Servidor | Ferramentas |
|---|---|
| `factory-triage`, `factory-architect` | `move_card`, `set_card_fields` |
| `factory-developer`, `factory-reviewer` | `move_card` |
| `factory-qa` | `move_card`, `record_qa_verdict` |

Um registro que não oferece nada, ou que oferece outra linha, recebeu o App
errado.

**Rode o canário do operador antes de iniciar qualquer papel.** Nenhum teste
prova o que o GitHub faz com um token restrito a um repositório, então, uma
vez por App de papel, com um token assim, rode as três chamadas do
[canário de setup](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/mcp/README.md#setup-canary-one-real-query-and-one-real-mutation)
contra um card descartável: a consulta `viewer { databaseId }` precisa
responder o id do bot daquele papel, a consulta do item precisa devolver o
projeto e o Status do card, e a mutação que move precisa responder o item sem
erros. Um papel cujo canário falha continua parado.

## 4. Classifique as ferramentas

Com cada servidor respondendo, classifique todas as ferramentas que os papéis
declaram (veja [O que a plataforma para antes de acontecer](what-the-gate-stops.md)).

**Toda ferramenta de leitura é não confiável (untrusted)** — não só as que
leem o corpo de uma issue. Uma listagem devolve texto escrito por pessoas: o
`projects_list` devolve os títulos das issues e pull requests do board mesmo
quando não se pede nenhum campo (#220).

| Classifique como | Ferramentas |
|---|---|
| read, **untrusted** | `projects_list`, `projects_get`, `search_pull_requests`, `search_issues`, `list_issues`, `issue_read`, `pull_request_read`, `get_file_contents`, `search_code`, `list_commits`, `get_commit` |
| write | `add_issue_comment`, `issue_write`, `pull_request_review_write`, `create_branch`, `create_or_update_file`, `push_files`, `create_pull_request` |
| write, resultado **não** untrusted | `move_card`, `set_card_fields`, `record_qa_verdict`, nos servidores `factory-<papel>` |

Classifique cada ferramenta no servidor de cada papel:
`github-triage.projects_list` e `github-reviewer.projects_list` são entradas
separadas. O `issue_write` também é como o Arquiteto cria uma sub-issue embaixo
da issue do card, então ele continua sendo write no `github-architect`.

O toolset `repos` do Arquiteto também oferece escritas de arquivo e de branch,
e nenhum header tira essas ferramentas no mesmo servidor: o `X-MCP-Readonly`
tiraria também as escritas em issues. A referência dele não declara nenhuma
delas, e o Contents: read do App dele as recusa.

As ferramentas da fábrica só respondem estado estruturado — motivos de um
conjunto fechado, nomes de coluna e de campo, números, SHAs de commit —, nunca
texto que alguém escreveu no GitHub, então o resultado delas não é untrusted.
Mesmo assim são escritas: uma pessoa aprova cada card movido, cada campo e
cada veredito do QA.

Deixe o `projects_write` **sem classificação**. Nenhum papel o declara, ele
fica fora de todas as superfícies, e uma ferramenta sem classificação nunca
executa.

## 5. Crie a área da fábrica

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
| Arquiteto | toda hora, aos :20 | 2.000.000 micros | 80 | 160 |
| Revisor | toda hora, aos :15 | 2.000.000 micros | 80 | 160 |
| QA | toda hora, aos :45 | 1.000.000 micros | 50 | 100 |
| Desenvolvedor | toda hora, aos :30 | 3.000.000 micros | 100 | 200 |

Micros são milionésimos da moeda configurada na instalação. Trate esses
valores como uma primeira calibração.

## 6. Crie os papéis

Para cada papel, crie o agente no Studio a partir da referência: os campos do
frontmatter, e o corpo como instruções.

- [Triagem](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/triage.agent.md)
- [Arquiteto](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/architect.agent.md)
- [Revisor](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/reviewer.agent.md)
- [QA](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/qa.agent.md)
- [Desenvolvedor](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/agents/developer.agent.md)

Depois publique — um agente novo é um rascunho pausado —, ponha o estágio em
**copiloto** e inicie. O estágio não está no arquivo, então ele sempre é
definido no console. Só inicie o Desenvolvedor depois que o `main` tiver o
ruleset mínimo (passo 2).

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

## O que o Arquiteto faz

O Arquiteto pega o card mais antigo da coluna Triagem com o rótulo
`factory:needs-design`, que a Triagem deixa para ele. Ele confere no código
cada afirmação sobre o código, só com ferramentas de leitura, e escreve o
plano e os critérios de aceite num comentário na issue: o corpo da issue
continua sendo de quem pediu, e um plano novo é um comentário novo, não uma
edição. Um item grande é dividido em sub-issues, criadas embaixo da issue do
card. Depois:

- sem nenhuma decisão em aberto, ele tira o `factory:needs-design` e move o
  card para Preparado, de onde uma pessoa o puxa para Doing;
- quando uma decisão cabe ao dono, ele escreve o plano até ali e cada decisão
  em aberto como pergunta, mencionando com @ quem decide para que o GitHub
  avise essa pessoa, e move o card para Travado mantendo o rótulo, para que o
  card volte para ele quando a pergunta for respondida e quem opera o board o
  devolver para Triagem, como diz a última linha do comentário (abaixo);
- se uma sub-issue, o rótulo ou a movimentação não puder ser gravado, ele diz
  qual na issue e move o card para Travado, para que o card não seja escolhido
  de novo a cada hora.

Ele nunca escreve código, nunca abre branch nem pull request e nunca planeja
uma mudança em `factory/` ou nos arquivos `CLAUDE.md`. O App dele não precisa
de ruleset no `main` antes de começar: com Contents: read, ele não consegue
escrever lá.

## O que o Desenvolvedor faz, e o que fica esperando

O Desenvolvedor trabalha no **Nível 1**: mudanças pequenas e autocontidas,
feitas pela API do GitHub, sem build e sem rodar testes antes do pull
request — uma correção de documentação ou de configuração, uma renomeação
mecânica, um patch num arquivo só, a versão fixada de uma dependência cujo
arquivo de lock não precisa de ferramenta para ser gerado de novo. Quando há
código envolvido, ele faz primeiro o commit do teste e abre o pull request
nesse commit, para que o CI mostre o teste falhando antes da mudança. Ele
põe o rótulo `factory:needs-review` no pull request e, quando a revisão o
devolve com `factory:changes-requested`, responde a cada apontamento com um
commit e um comentário.

Um card que precisa de um build ou de rodar testes antes do pull request não
é tentado. O Desenvolvedor põe o rótulo `factory:needs-runtime` na issue,
explica o motivo num comentário e deixa o card em Doing para uma pessoa. Se o
rótulo não puder ser aplicado, porque nunca foi criado (passo 1), ele avisa e
move o card para Travado, para que o card não seja escolhido de novo a cada
hora. O **Nível 2**, build e testes antes do pull request, virá de um runtime de
código atrás de uma única ferramenta MCP: o runtime previsto é o
[FuseOne Forja](https://github.com/fuseone-io/fuseone-forja), um projeto à
parte, ainda na fase 0 proposta, com `forja.implement` como ferramenta de
escrita sob o Gate.

## Quem tira um card de Travado

Nenhum papel tira um card de Travado, e resolver o que o travou também não
move o card. Na primeira versão isso é uma tarefa fixa de uma pessoa,
dada a quem opera o board:

Quem opera o board revisa a coluna Travado todo dia útil e, quando o bloqueio indicado no comentário que travou um card é resolvido, devolve o card para a coluna que esse comentário indica, Triagem ou Doing, sem mexer nos rótulos; esse comentário é o mais recente do GitHub App do papel que travou o card que termina com uma linha Return to:, na issue ou no pull request em que o papel trabalhou, sendo o pull request o próprio card ou o que aparece no campo Linked pull requests do card.

Um card não volta para nenhuma coluna quando o item que ele representa terminou: quem opera o board o move de Travado para Done quando o card é uma issue fechada, ou um pull request com merge feito ou fechado.

Um pull request fechado sem merge enquanto a issue do card continua aberta não encerra o card: ele volta para a coluna indicada pela linha Return to:, e uma pessoa começa a próxima tentativa, já que o Desenvolvedor pula um card que tem pull request vinculado.

A Triagem e o Arquiteto escrevem esse comentário na issue do card; o
Revisor, o QA e o Desenvolvedor, quando responde a uma revisão, escrevem no
pull request, que é o próprio card ou o que aparece no campo **Linked pull
requests** do card. A última linha desse comentário é fixa: `Return to: Triagem` quando quem
travou o card foi a Triagem ou o Arquiteto, `Return to: Doing` quando foi o
Desenvolvedor, o Revisor ou o QA. Um card que volta para Triagem ainda com
`factory:needs-design` é retomado pelo Arquiteto; um card que volta para Doing
mantém o rótulo do pull request, e o papel indicado por esse rótulo o
retoma.

O papel que travou o card diz nesse comentário o que bloqueia o card e o que
uma pessoa precisa fazer para resolver — responder a uma pergunta, criar um
rótulo que falta, dizer se um plano pela metade deve ser retomado ou
descartado —, nunca só o que aconteceu. É essa ação que impede o papel de
travar o card de novo no próximo run; quem opera o board é quem percebe que
ela foi feita. O
[LANES](https://github.com/fuseone-io/fuseone-agents/blob/main/factory/LANES.md)
lista cada caminho para Travado e a ação de cada um. Sem alguém nessa tarefa,
um card travado fica esperando para sempre.

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
