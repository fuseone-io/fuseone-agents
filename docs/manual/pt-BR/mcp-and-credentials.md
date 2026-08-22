---
title: MCPs e credenciais
summary: Como conectar servidores, escolher superfície, classificar ferramentas e decidir quem fornece a credencial.
section: integrations
tags: mcp, credencial, superfície, classificação, oauth, stdio, local
order: 5
---

## O catálogo é receita, não conector

Uma receita diz como um servidor costuma ser alcançado, quais credenciais ele
espera e quais ferramentas a documentação publica. Ela não é homologação e não
aplica confiança sozinha.

Conectar não classifica. Classificar não traz ferramenta para a superfície.
Credencial não estreita alcance. São três decisões diferentes.

## Transporte

| Transporte | Quando usar | Cuidado |
|---|---|---|
| HTTP remoto | Preferido em produção | A plataforma envia só a credencial configurada |
| stdio local | Útil para desenvolvimento ou servidor sem remoto | Roda como o worker, dentro do container do worker |

Servidor stdio não herda o ambiente do worker. Ele recebe só a allowlist da
plataforma e as variáveis configuradas para ele. Se precisar de arquivo, a
plataforma sela o conteúdo, materializa em diretório 0700 com arquivo 0600, e
remove quando o processo termina.

## Superfície

A superfície diz quais ferramentas deste servidor existem para esta
instalação. Fora da superfície a ferramenta não é proibida: ela não existe para
os agentes.

Uma ferramenta nova em servidor com superfície escolhida nasce fora. Isso é o
comportamento seguro: novidade não entra no alcance por acidente.

## Classificação

Cada ferramenta precisa de uma decisão do Curador:

- efeito: read, write, destructive ou financial;
- se o resultado é untrusted;
- qual ferramenta compensa, quando houver.

A decisão carrega o digest da definição julgada. Se o servidor mudar schema ou
descrição da ferramenta, a decisão antiga fica stale e o Gate recusa até nova
revisão.

## Credencial da instalação

Use credencial da instalação quando o servidor age como uma conta de serviço:
discovery, health checks, probes e ferramentas que não representam uma pessoa
específica.

Essa credencial é compartilhada por todas as runs que usam o servidor nesse
modo. Se o token alcança a conta inteira, a plataforma não consegue estreitar
isso; a superfície e a classificação só controlam o que os agentes podem
tentar.

## Credencial pessoal

Use credencial pessoal quando o servidor espera autoridade de usuário: Google
Workspace, Slack OAuth, GitHub pessoal, Atlassian e casos parecidos.

A credencial é selada por servidor e principal. Uma run só consegue usá-la se
carrega `OnBehalfOf` daquele principal. Runs agendadas não têm pessoa; se a
receita diz que a credencial é pessoal, elas param em vez de cair para a
credencial da instalação.

## Formas de autenticação

O catálogo declara a forma, mas o runtime só envia o que foi configurado.

| Forma | O que configurar |
|---|---|
| Bearer | Token simples |
| Header | Um ou mais headers exatos exigidos pelo servidor |
| OAuth | Access token, refresh token, token URL, client id e secret |
| Basic | Header pronto, quando o servidor espera Basic |
| DSN | String de conexão |
| Env | Variáveis para stdio |
| Config file | Documento selado materializado no worker |

Não preencha duas formas ao mesmo tempo. A tela recusa para evitar que uma
pessoa configure uma coisa e o runtime envie outra.

## Cache de resultado

Um servidor pode cachear resultados de leitura bem-sucedidos por um TTL curto.
Use para leituras estáveis e repetidas, como "listar datasources" ou "buscar o
mesmo runbook", não para dados que precisam estar frescos em toda chamada.

A cache vive em cada processo de worker, não em um store compartilhado. Com
dois workers, qualquer um deles pode dar miss até ter visto a mesma leitura. Um
hit ainda grava uma referência de conteúdo nova para a run atual, e a trilha
diz qual run e passo anteriores forneceram os bytes.

A chave inclui definição da ferramenta, argumentos, escopo e `OnBehalfOf`.
Assim um resultado produzido com credencial pessoal fica com a pessoa que o
produziu.

## Diagnóstico rápido

- Servidor "answering" mas tool falha com credencial pessoal: configure a
  credencial na aba Credentials para o usuário que iniciou a run.
- Tool aparece classificada mas Gate recusa: confira digest stale ou se a
  ferramenta está fora da superfície.
- HTTP MCP falha com protocolo: altere o protocol mode do servidor para legacy
  quando a receita ou o servidor exigir.
- stdio não inicia depois de upgrade: aceite execução local explicitamente.
