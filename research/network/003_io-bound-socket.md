# Operações I/O-bound em rede: pools de conexão e sockets TCP/IP

## O que define uma operação I/O-bound de rede

Uma operação é I/O-bound quando o gargalo é **esperar** — não computar. Em rede, isso significa que a thread/goroutine passa a maior parte do tempo bloqueada em syscalls como `read()`, `write()`, `connect()`, aguardando bytes chegarem da NIC ou confirmação do peer. CPU fica ociosa. A latência é dominada por RTT, throughput da rede e o overhead do kernel mediando o socket.

Implicação prática: paralelismo aqui não é sobre mais cores, é sobre **mais conexões em voo simultaneamente**. É por isso que Go (goroutines + netpoller) e Node (event loop + epoll/kqueue) brilham aqui — escalam conexões sem escalar threads do SO.

## Anatomia de um socket TCP/IP

Um socket é um file descriptor que o kernel associa a uma 4-tupla: `(IP origem, porta origem, IP destino, porta destino)`. Cada conexão única consome:

- Um FD no processo (limite via `ulimit -n`)
- Buffers de envio/recepção no kernel (tipicamente 64KB-4MB cada, ajustáveis via `SO_SNDBUF`/`SO_RCVBUF`)
- Estado de controle (sequence numbers, congestion window, RTT estimates)

O custo de **abrir** uma conexão TCP não é trivial:

1. **Three-way handshake**: SYN → SYN-ACK → ACK. Custa 1 RTT antes de qualquer byte útil.
2. **TLS handshake** (se aplicável): +1-2 RTTs adicionais, mais operações criptográficas (CPU spike momentâneo).
3. **Slow start**: TCP começa com congestion window pequena e cresce exponencialmente. Conexões novas transferem dados mais devagar nos primeiros KBs.
4. **TIME_WAIT**: ao fechar, o socket fica em TIME_WAIT por ~60s ocupando a 4-tupla. Em alto volume, isso esgota portas efêmeras (range típico 32768-60999).

Conclusão: conexão é **cara de criar** e **cara de descartar**. É exatamente esse fato que justifica pools.

## Pools de conexão: por que existem

Um pool mantém um conjunto de conexões TCP já estabelecidas (e idealmente já com TLS handshake feito) prontas para reuso. Quando seu código pede uma conexão, o pool entrega uma existente em vez de abrir nova.

O ganho vem de:

- **Amortização de handshake**: o RTT inicial paga-se uma vez por conexão, não por requisição.
- **Congestion window aquecida**: conexões reusadas já estão fora do slow start.
- **Pressão reduzida em TIME_WAIT**: menos open/close churn.
- **Backpressure natural**: se o pool tem limite (digamos 100 conexões) e todas estão ocupadas, requisições novas esperam — isso protege o servidor downstream de overload.

## Parâmetros que importam num pool

Os trade-offs centrais que você ajusta:

- **MaxOpen / MaxConns**: teto de conexões simultâneas. Muito alto → derruba o servidor remoto e/ou esgota FDs locais. Muito baixo → contenção, requisições enfileiram.
- **MaxIdle**: quantas conexões ficam ociosas no pool prontas para reuso. Idle demais desperdiça FDs e memória; pouco demais causa reabertura constante.
- **IdleTimeout**: quanto tempo uma conexão idle vive antes de ser fechada. Importante porque firewalls/load balancers cortam conexões silenciosamente após N minutos — usar uma conexão "morta" gera erro só na próxima escrita.
- **MaxLifetime**: tempo máximo absoluto de vida de uma conexão, mesmo que esteja sendo usada. Útil para forçar rebalanceamento quando há DNS round-robin ou rolling deploys do upstream.
- **ConnectTimeout / ReadTimeout / WriteTimeout**: limites por operação. Sem eles, uma conexão lenta trava uma goroutine indefinidamente.

No `net/http` do Go, isso aparece em `http.Transport`: `MaxIdleConns`, `MaxIdleConnsPerHost`, `MaxConnsPerHost`, `IdleConnTimeout`. O default `MaxIdleConnsPerHost = 2` é uma armadilha clássica — em cliente HTTP de alto volume, você quase sempre quer aumentar.

## Onde isso quebra na prática

Alguns padrões de falha que aparecem em produção:

**Connection leak**: você pega conexão do pool e não devolve (esquece `defer rows.Close()` em SQL, esquece `resp.Body.Close()` em HTTP). Pool esvazia, novas requisições bloqueiam, parece deadlock.

**Half-open connections**: o peer caiu mas TCP não detectou (sem keepalive ou keepalive longo). Você manda dados, espera ACK, timeout só dispara minutos depois. `SO_KEEPALIVE` mitiga, mas o intervalo default do Linux é 2h — geralmente você quer reduzir.

**Pool subdimensionado para latência alta**: se cada request leva 100ms e você precisa de 1000 req/s, precisa de ~100 conexões simultâneas no mínimo (Little's Law: L = λW). Pool de 10 vai estrangular o throughput independentemente de quão rápida sua aplicação seja.

**Thundering herd no startup**: sem pool pré-aquecido, o primeiro burst de tráfego abre N conexões simultâneas, cada uma pagando handshake completo, e você vê um spike de latência no boot.

## Resumo da relação

I/O-bound de rede é dominado por latência de espera, não por CPU. O socket TCP é a unidade de custo — caro de abrir, caro de manter, caro de descartar. O pool de conexões é a abstração que transforma esse custo fixo em custo amortizado, e seus parâmetros (max, idle, timeouts) são onde você equilibra utilização de recursos locais contra pressão no peer remoto e contra latência de cauda.