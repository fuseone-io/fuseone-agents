---
title: Automatizando a configuração
summary: Como um pipeline lê um recurso, descobre se ele mudou e o grava sem sobrescrever a mudança de outra pessoa, e o que a API se compromete a não quebrar.
section: reference
tags: api, automação, terraform, pipeline, etag, if-match, 412, compatibilidade, mudança incompatível
order: 26
---

## Para quem é

Para quem mantém a configuração de uma instalação num repositório e a aplica
por um pipeline, um script ou uma execução de Terraform. O console usa a mesma
API, com as mesmas regras. Assim, quando uma pessoa e um pipeline editam o mesmo
recurso, cada um vê a mudança do outro em vez de apagá-la.

Um pipeline age por uma conta de serviço (veja
[Empresas, áreas e papéis](scopes-and-roles.md)). Todos os caminhos abaixo
ficam sob `/api/v1`, e o contrato é o `api/openapi.yaml` do repositório.

## O que dá para gerenciar assim

| Recurso | Caminho | Exige |
|---|---|---|
| Política | `/policies/{code}` | `policy:write` |
| Servidor de ferramentas | `/admin/integrations/mcp-servers/{name}` | `provider:write` |
| Provedor de modelo | `/admin/integrations/providers/{name}` | `provider:write` |
| Canal | `/admin/channels/{name}` | `provider:write` |
| Instância de conector | `/admin/integrations/connectors/instances/{name}` | `provider:write` |

Cada um aceita `GET`, `PUT` e `DELETE` pelo nome. Uma instância de conector
também é nomeada dentro de um escopo: `GET` e `DELETE` recebem `scopeKind` na
query, mais `company` e `area` conforme o escopo pede, e o `PUT` leva o mesmo no
corpo. Sem isso, a requisição é 400. A permissão é verificada onde
fica a administração, então o papel precisa ter sido concedido ali; hoje Admin e
Curador têm as duas. Ler exige a mesma permissão que gravar, exceto no caso de
uma política, que qualquer `policy:read` basta para ler: uma regra que barra
você tem que ser uma regra que você consegue ler.

## Ler de volta o que foi gravado

O `GET` responde o que o `PUT` guardou. Um segredo nunca volta: a resposta diz
apenas se há um guardado (`hasSecret`, `hasKey`, `hasToken` e afins). Um
servidor de ferramentas, um provedor de modelo e um canal também trazem um
`secretFingerprint`, com chave, que muda quando um segredo muda e não revela
mais nada sobre ele. Uma instância de conector não traz nenhum; o único sinal de
que o token dela mudou é o `ETag`, que nomeia o recurso inteiro. A resposta
também traz o que o servidor mantém para si, como `updatedAt` e `updatedBy`, que
uma gravação não envia.

Então o cliente compara os campos não secretos que o seu `PUT` envia com os
mesmos campos da resposta do `GET`, e uma diferença é mudança de verdade. Saber
se o segredo que ele tem é o que está guardado é pergunta para a impressão
digital, onde houver uma, e senão para o `ETag` que ele guardou da última
gravação.

**Gravar o que já está guardado não muda nada.** Nenhum `updatedAt` novo,
nenhuma linha na trilha, nenhum snapshot novo de política, e volta o mesmo
`ETag`. Um pipeline pode aplicar a configuração inteira a cada execução sem
deixar rastro onde nada mudou.

## Gravar sem sobrescrever outra pessoa

Todo `GET` acima responde um `ETag`, e todo `PUT` responde o que deixou.
Devolva-o como `If-Match` no próximo `PUT` ou `DELETE`:

```http
GET /api/v1/admin/integrations/providers/gateway
→ 200, ETag: "v1-3f9c…"

PUT /api/v1/admin/integrations/providers/gateway
If-Match: "v1-3f9c…"
→ 204, ETag: "v1-77ab…"   gravado
→ 412                     alguém mudou antes; nada foi gravado
```

- **Um 412 não gravou nada.** O tipo do problema é
  `fuseone:precondition-failed`, e o cabeçalho `ETag` diz o que o recurso é
  agora. Não há `ETag` quando o recurso deixou de existir. Leia de novo e decida
  com o que está lá; nunca reenvie o mesmo corpo com o tag novo sem olhar.
- **`If-Match: *`** quer dizer "só se existir". Qualquer `If-Match` num recurso
  que não existe é 412, `*` incluído.
- **Tags fracos (`W/"…"`) nunca casam.** Um cabeçalho que não se consegue ler é
  400, nunca ignorado: ignorá-lo desligaria a proteção que o cliente pediu.
- **Sem `If-Match`, vale a última gravação**, como sempre foi.

O `ETag` nomeia o conteúdo do recurso, não o momento em que foi gravado. Ele
muda quando a configuração ou um segredo guardado muda. O de um canal também
muda quando uma conversa ou uma identidade é vinculada a ele, e o de um servidor
de ferramentas quando alguém cola uma concessão OAuth, mas não quando um worker
a renova. Trocar a chave mestra da instalação muda o `ETag` de todo recurso que
guarda um segredo. Trate o tag como opaco: compare, nunca interprete.

## O que a API se compromete a não quebrar

A API é um contrato público. O console, os pipelines e as integrações que um
cliente escreve usam todos os mesmos caminhos.

**Não é mudança incompatível**, e é anunciado em **Added** ou **Changed**:

- um caminho novo, uma operação nova ou um campo opcional novo numa requisição;
- um campo novo numa resposta. Um cliente tem que ignorar campos que não
  conhece;
- um tipo de problema novo para a recusa de algo novo, como um valor de um
  campo novo ou uma requisição a um caminho novo. Ele não transforma em recusa
  uma requisição que antes funcionava nem substitui o tipo de uma recusa que já
  existe; qualquer um dos dois é mudança incompatível, abaixo.

**É mudança incompatível:**

- remover ou renomear um caminho, uma operação ou um campo, ou mudar o tipo de
  um campo ou o que ele significa;
- tornar obrigatório um campo opcional da requisição, ou recusar uma requisição
  que antes funcionava. A plataforma faz isso de propósito quando uma regra de
  governança passa a valer, e continua sendo mudança incompatível, anunciada do
  mesmo jeito;
- mudar o status ou o tipo de problema de uma recusa com que um cliente age;
- mudar o que uma requisição sem `If-Match` faz;
- mudar como um `ETag` é feito. Todo tag começa pelo seu formato, `v1-`, então
  uma mudança assim aposenta todos os tags antigos de forma visível: um cliente
  que guarda um recebe 412 e lê de novo, e nunca um casamento que quer dizer
  outra coisa.

**Como uma mudança incompatível é anunciada.** Nas **Upgrade notes** da versão
que a traz, que vêm primeiro no [changelog](https://github.com/fuseone-io/fuseone-agents/blob/main/CHANGELOG.md),
dizendo o que deixa de funcionar e o que fazer no lugar. Nunca numa versão de
correção, a não ser para fechar uma falha de segurança, e aí as notas dizem
isso.

**Tipos de problema nunca mudam depois de publicados.** Decida pelo `type`,
nunca pelo `title` ou pelo `detail`: o título é um texto de reserva em inglês, e
o detalhe traz particularidades como um identificador ou uma permissão.

**Durante um rollout, duas versões respondem.** Um pod ainda na versão anterior
pode atender uma requisição enquanto a nova é implantada. As notas de upgrade
dizem quando um cliente deve esperar todas as réplicas antes de contar com algo
novo, como disseram para o próprio `If-Match` na 0.97.0.
