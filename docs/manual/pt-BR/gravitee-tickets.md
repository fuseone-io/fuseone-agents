---
title: Chamados governados no Gravitee
summary: Transforme uma thread de suporte no Slack numa aceitação de assinatura Gravitee inspecionada, aprovada e recuperável.
section: integrations
tags: gravitee, slack, chamado, api key, aprovação, vault
order: 18
---

## O que esse fluxo faz

Um desenvolvedor vinculado abre uma mensagem raiz que casa numa sala de suporte
configurada no Slack. O FuseOne mantém aquela thread como um chamado, deixa o
solicitante corrigi-lo, inspeciona a assinatura pendente no Gravitee e pede a
um gestor nomeado que aprove a aceitação exata. Depois de uma decisão válida,
aceita a assinatura uma vez e responde na thread original.

A resposta informa assinatura e expiração e lembra o solicitante de armazenar
e rotacionar a credencial com segurança. Ela **nunca contém a API key** nem
nomes de exibição remotos. O solicitante recupera a chave no Gravitee pelo
caminho protegido da organização.

## Antes de configurar a sala

1. Configure um conector Gravitee habilitado na mesma empresa e área da sala de
   suporte. Vincule exatamente uma API, uma organização, um ambiente e a faixa
   de expiração permitida.
2. Vincule o conector a uma credencial apoiada pelo Vault com somente leitura e
   aceitação de assinaturas naquele escopo remoto. O FuseOne resolve o segredo
   internamente; o modelo e o Slack nunca o recebem.
3. Publique um agente com trigger de conversa e acesso ao conector. A instrução
   deve coletar id da assinatura, motivo e expiração, inspecionar primeiro e
   chamar somente a ferramenta registrada de aceitação.
4. Vincule as contas Slack do solicitante e do aprovador a pessoas no FuseOne.
   Dê ao aprovador um papel com `approval:act` num escopo que contenha o chamado.
5. Dê ao app Slack `message.channels`, convide-o para a sala, configure
   Interactivity para os botões e conceda `im:write` para mensagens diretas.

A primeira versão autoriza somente o **dono principal** da aplicação como
solicitante. Um membro da aplicação que não seja o dono principal não pode abrir
essa aceitação, mesmo que tenha criado a assinatura.

## Configure a conversa

Em **Integrações -> Canais**, adicione ou edite a sala de suporte e escolha
**Chamados governados**. Selecione escopo e agente, escolha o principal de
serviço usado como `run as`, informe a fonte de endereçamento exata como
`bot:<id>` ou `app:<id>` e adicione padrões RE2 estreitos que identifiquem os
pedidos suportados.

`run as` é o principal de execução; não é o solicitante. A pessoa vinculada que
escreveu a raiz continua sendo `RequestedBy`, e o bot ou app configurado apenas
escolhe quem será avisado. Nenhuma dessas identidades substitui quem aperta
Aprovar.

## Valide com segurança

1. Crie uma assinatura de teste pendente na única API Gravitee configurada.
2. Publique um pedido raiz que case, a partir de uma conta Slack vinculada,
   incluindo id da assinatura e expiração desejada.
3. Confirme que uma run de chamado abre no escopo da sala e que o texto original
   está marcado como não confiável.
4. Faça o bot ou app de endereçamento mencionar um aprovador real na thread.
   Confirme o card sob a raiz e na DM dessa pessoa.
5. Abra a aprovação e compare toda a evidência com o Gravitee. Altere a
   assinatura remota antes de aprovar uma vez; o FuseOne deve recusar o snapshot
   antigo sem enviar a aceitação.
6. Repita com uma assinatura nova e aprove. Confirme uma única aceitação no
   Gravitee e a resposta segura na thread, sem chave.
7. Remova o grant ou o vínculo de conta do aprovador e repita. A DM não deve ser
   entregue e o nome no chamado não pode conceder autoridade.

Se o processo parar depois que o Gravitee pode ter aceitado o pedido, não envie
de novo manualmente antes de conferir. O worker de recuperação reconcilia a
tentativa gravada contra a assinatura e nunca repete um POST ambíguo
automaticamente. Se não puder provar o resultado dentro da janela limitada, a
thread pede verificação manual enquanto o FuseOne mantém a execução reservada
e continua observando somente por GET. Um estado confirmado depois disso é
publicado na mesma thread.

Depois de conferir a assinatura no Gravitee, um operador com `run:cancel` pode
usar **Abandonar run** para interromper a observação. Essa decisão explícita
libera o chamado para uma revisão posterior; ela não desfaz uma aceitação que o
Gravitee talvez já tenha realizado, então registre o motivo e confira o estado
remoto primeiro.
