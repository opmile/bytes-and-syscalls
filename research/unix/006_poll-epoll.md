# Entendendo Poll e Epoll: A Base do Netpoller do Go

O desafio central da programação de rede de alta performance é: **Como um único processo pode monitorar milhares de conexões de rede (File Descriptors ou fds) sem travar o sistema?**

A forma como o sistema operacional responde a essa pergunta muda completamente a eficiência do software. É essa diferença que sustenta a capacidade do Go de lidar com milhões de conexões simultâneas.

---

## 1. Poll: A Abordagem Direta (Mas Ineficiente)

O `poll` funciona como uma "lista de chamadas" estática. Toda vez que você quer saber se há dados para ler ou espaço para escrever, você precisa enviar a lista completa de conexões para o Kernel do sistema operacional.

### Como o `poll` opera:

1. Você envia um array com **todos** os fds para o Kernel.
2. O Kernel varre a lista um por um para registrar o interesse.
3. O Kernel bloqueia o processo até que algo aconteça.
4. Quando há dados, o Kernel varre a lista toda novamente para marcar quem está pronto.
5. Você recebe a lista de volta e precisa varrê-la inteira no seu código para descobrir quais fds mudaram.

> **O problema do $O(N)$:** Se você tem 10.000 conexões abertas e apenas 3 recebem dados, o `poll` vai copiar e varrer as 10.000 estruturas de dados na ida e na volta, a cada ciclo. Isso destrói a performance em escala.

---

## 2. Epoll: Eficiência com Estado no Kernel

A grande inovação do `epoll` é **manter a lista de conexões salva dentro do Kernel**. Você não precisa reenviar todas as conexões a cada teste; você apenas gerencia essa lista através de três funções básicas:

* `epoll_create1`: Cria uma instância do epoll no Kernel.
* `epoll_ctl`: Adiciona, modifica ou remove conexões da lista de monitoramento (**Interest List**). Você faz isso apenas uma vez por conexão.
* `epoll_wait`: Pergunta ao Kernel quais conexões estão prontas.

O Kernel gerencia internamente duas estruturas dinâmicas:

* **Interest List (Lista de Interesse):** Todas as conexões que você pediu para monitorar.
* **Ready List (Lista de Prontos):** Apenas as conexões que realmente têm dados disponíveis. Essa lista é populada diretamente pelos drivers de rede.

> **A vantagem de Escala:** Com as mesmas 10.000 conexões e 3 ativas, o `epoll_wait` retorna instantaneamente **apenas as 3 conexões prontas**. Não há varredura de itens ociosos.

---

## 3. Gatilhos de Notificação: Level-Triggered vs. Edge-Triggered

O `epoll` pode avisar você sobre eventos de duas formas distintas:

### Level-Triggered (LT) — Modo Padrão

Funciona como um sensor de nível de água. Enquanto houver dados no buffer da conexão, o Kernel continuará avisando que ela está pronta em cada chamada do `epoll_wait`.

### Edge-Triggered (ET) — Modo de Borda

Funciona como um sensor de movimento. O Kernel avisa você **apenas uma vez**, no momento exato em que novos dados chegam. Se você ler apenas metade dos dados e chamar o `epoll_wait` de novo, o Kernel não vai te avisar sobre o resto até que cheguem novos bytes.

* **Regra de ouro do ET:** Exige sockets **não-bloqueantes** e um loop que consuma os dados (via `read` ou `write`) até receber o erro `EAGAIN` (que significa "não há mais nada para ler por enquanto").
* **Vantagem:** É mais rápido e evita notificações redundantes no sistema, além de prevenir problemas como o *thundering herd* (múltiplas threads acordando para o mesmo evento).

---

## 4. Por que isso importa para o Go? (O Netpoller)

Quando você escreve um código simples em Go, como `conn.Read(buf)`, parece que a sua Goroutine trava e fica esperando os dados ali. Mas por baixo dos panos, o Runtime do Go faz algo muito mais inteligente:

1. O socket real é configurado em modo **não-bloqueante**.
2. A Goroutine tenta ler os dados. Se a leitura retornar dados imediatamente, o fluxo segue.
3. Se retornar `EAGAIN` (sem dados no momento), o Runtime do Go entra em ação: ele pausa (**park**) a Goroutine atual e registra o fd no `epoll` global do Go (o **Netpoller**), usando o modo **Edge-Triggered**.
4. A thread do sistema operacional (M) fica livre para rodar qualquer outra Goroutine produtiva.
5. Quando o `epoll_wait` do Netpoller detecta que a conexão recebeu dados, o Go acorda a Goroutine pausada e a devolve para a fila de execução.

Graças a essa arquitetura, milhares de conexões de rede custam apenas alguns bytes na memória do Kernel e utilizam pouquíssimas threads do sistema operacional.

---

## Resumo Comparativo

| Característica | Poll | Epoll |
| --- | --- | --- |
| **Estado das Conexões** | Reenviado a cada chamada. | Mantido na memória do Kernel. |
| **Complexidade (Custo)** | $O(N)$ — Proporcional ao total de conexões. | $O(\text{prontos})$ — Proporcional apenas às conexões ativas. |
| **Modos de Notificação** | Apenas Level-Triggered. | Level-Triggered ou Edge-Triggered (Usado pelo Go). |
| **Escalabilidade** | Ruim para mais de 1.000 conexões. | Excelente (Projetado para mais de 10.000 conexões). |
| **Portabilidade** | Padrão POSIX (Funciona em quase tudo). | Exclusivo do Linux. |

---

## Próximos Passos Recomendados

Para consolidar esse conhecimento antes de construir servidores do zero, siga esta trilha no código-fonte do Go:

1. **`src/runtime/netpoll.go`**: Analise o contrato genérico do netpoller. Foque em entender as funções `netpollblock` (pausar a Goroutine) e `netpollready` (acordar a Goroutine).
2. **`src/runtime/netpoll_epoll.go`**: Veja a implementação real para Linux. Procure pelas chamadas diretas de sistema: `epoll_create1`, `epoll_ctl` e `epoll_wait`.
3. **Prática**: Tente criar um pequeno programa usando o pacote `golang.org/x/sys/unix` para manipular o `epoll` manualmente. Aceite conexões, configure-as com `EPOLLET` (Edge-Triggered) e trate o retorno de `EAGAIN`. Isso removerá qualquer "magia" de como o Go escala por baixo dos panos.