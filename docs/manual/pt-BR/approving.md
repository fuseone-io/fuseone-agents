---
title: Aprovar uma ação
summary: O que você está decidindo quando uma execução para, e quando a resposta certa é mudar a política em vez de aprovar.
section: operations
tags: aprovação, fila humana, decidir, recusar, escalar
order: 10
---

## O que parou e por quê

Quando uma execução para, ela aparece na **Fila humana**. A tela mostra três coisas, e a primeira é a que importa:

**Os argumentos exatos** que serão enviados. Não uma descrição do que o agente pretende fazer — o conteúdo real da chamada.

**A regra que parou.** Pode ser a escada de efeito, o rastro de conteúdo externo, ou uma política com código e motivo.

**O que a execução fez até ali.** A trilha inteira, para você entender o caminho e não só o último passo.

## Aprovar libera aquela chamada

Aprovar vale para **aquela chamada, com aqueles argumentos**. Não liga a ferramenta, não vale para a próxima vez, não cria exceção. A execução seguinte para de novo.

É por isso que ler os argumentos é o trabalho, e não uma formalidade. A pergunta não é *"esse agente é confiável?"* — é *"essa chamada específica deve acontecer?"*.

## Quando a resposta certa não é aprovar

**Aprovar dez vezes seguidas a mesma coisa é um sinal**, não uma rotina. Significa que a política daquela área está errada — ou porque para algo que deveria passar, ou porque quem aprova parou de decidir.

Nos dois casos o conserto é a política, não o clique.

E o contrário também vale: se uma chamada te surpreendeu, recuse e vá olhar a instrução do agente. **Recusar não é punir o agente** — é o mecanismo dizendo que algo não estava previsto.

## Recusar é informação

Uma recusa fica registrada com quem decidiu. Se a execução puder continuar sem aquela ação, ela continua; se não, ela para com a recusa na trilha.

Quem escreveu o agente vê isso e aprende o que o mundo real trouxe que o ensaio não trouxe.

## Casos de uso

### O conteúdo veio de um chamado

Uma execução leu o texto de um chamado e agora quer escrever. Ela para, porque o que veio de fora carrega marca.

**O que olhar:** se o que vai ser escrito reflete o chamado, ou se reflete algo que o texto do chamado *pediu* ao agente. A segunda é a tentativa de instrução dentro de conteúdo, e a resposta é recusar.

### O agente quer apagar alguma coisa

Efeito destrutivo é bloqueado por padrão. Se ele chegou até você, alguém abriu por política.

**O que olhar:** o alvo exato nos argumentos. Destrutivo é a única categoria em que não dá para desfazer depois de aprovar.

### Você não entende a chamada

Recuse. Uma aprovação que você não consegue explicar não é uma aprovação — é uma assinatura.

O que a plataforma para sozinha está em [O que a plataforma para antes de acontecer](what-the-gate-stops.md), e como escrever a regra que muda isso está em [Políticas](policies.md).
