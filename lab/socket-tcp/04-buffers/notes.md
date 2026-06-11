# Buffers, flow control e o bloqueio observável do `Write`

## Propósito da estação

Provar — quantitativamente — que `conn.Write` **não escreve na rede**. Escreve no
**send buffer do kernel** e retorna. Só bloqueia em condições específicas:
buffer cheio porque o receiver não está drenando rápido o suficiente.

O byte exato onde o `Write` congela vira evidência observável da existência
desses buffers e do flow control do TCP.

---

## Por que dois lados separados (server.go + client.go)

Pra ver `Write` bloquear, é preciso **quebrar de propósito o consumo** do
outro lado. Se ambos lessem normalmente, o buffer nunca encheria, o `Write`
nunca bloquearia e nada seria provado.

O experimento exige dois papéis opostos:

| Lado | Papel | Comportamento |
|------|-------|---------------|
| `server.go` | slow-reader (consumidor sabotado) | `Accept` → `Sleep(15s)` → drena em pulsos |
| `client.go` | write-burst (produtor agressivo) | `Dial` → `Write(4KB)` em loop até quota (64MB) → fecha |

Server fica 15 segundos **sem ler nada**. Cliente joga bytes o mais rápido
que conseguir.

### Por que dois binários, e não goroutines no mesmo processo?

1. **Isolamento de medida.** `ss -tn` mostra `Recv-Q` (lado server) e
   `Send-Q` (lado client) como linhas separadas. Se fossem o mesmo processo,
   funcionaria por loopback mas confundiria a leitura — quem é quem?
   Binários separados deixam óbvio.
2. **Realismo do cenário.** Em produção, sender e receiver costumam ser
   processos distintos (e frequentemente máquinas distintas). Separar
   reflete isso.
3. **Controle de timing.** Sobe o server primeiro (fica em `Accept`), abre
   o `watch` no inspector, **depois** dispara o cliente. Sequência
   reproduzível e fácil de observar ao vivo.

---

## Cadeia do flow control (o que acontece quando o cliente "congela")

```
1. Cliente Write(4KB) → send buffer do kernel do cliente
2. TCP do cliente envia → receive buffer do kernel do server
3. App do server NÃO chama Read (está em Sleep)
4. Receive buffer do server enche → ACK anuncia window=0
5. TCP do cliente para de enviar → send buffer do cliente enche
6. Próximo Write do cliente → BLOQUEIA (goroutine parkada pelo netpoller)
7. Contador no terminal congela em N bytes
```

`N` é aproximadamente:
```
N ≈ (receive buffer efetivo do server) + (send buffer efetivo do client)
```
Cada buffer fica em **algum ponto entre `default` e `max`** — o kernel faz
autotuning dinâmico conforme o uso, então qual ponta estaciona no `default` e
qual no `max` não é fixo. A fórmula `rmem.default + wmem.max` é um chute que
às vezes bate, não uma previsão firme. Os limites saem de:
```bash
sysctl net.ipv4.tcp_rmem
sysctl net.ipv4.tcp_wmem
```
Cada um devolve `min default max`.

---

## O que cada terminal mostra durante o experimento

- **Server:** imprime `"Dormindo 15s..."` → 15s depois → drena em pulsos (lê
  4MB, pausa 1s, repete) e segue até o client fechar (`EOF`); aí imprime
  `"Drenei N bytes em X"`. O loop só termina por causa do `EOF` — por isso o
  client tem quota finita e fecha.
- **Cliente:** contador sobe rápido (4KB, 1MB, 3MB...) → **congela** num
  número específico → 15s depois → volta a crescer (em degraus, no ritmo dos
  pulsos do server) → termina a quota, imprime `Escrevi ... Fechando conn` e fecha.
- **Inspector (`ss -tn`):** `Recv-Q` do server cresce e estaciona; `Send-Q`
  do cliente cresce e estaciona. A soma é o total que ficou retido entre os
  dois buffers do kernel.

---

## Conexão com o resto da família

- **Estação 03:** o netpoller (FDs `eventpoll`/`eventfd`) é exatamente o
  mecanismo que parka a goroutine do `Write` aqui. Sem netpoller, a thread
  do SO ficaria travada esperando o buffer abrir.
- **`004_concorrency.md`:** "API síncrona para a goroutine, assíncrona para
  a thread M". Esta estação prova: a goroutine do `Write` fica visivelmente
  parada (contador congelado), enquanto a thread M continua livre rodando
  outras coisas — inclusive a goroutine do contador que segue imprimindo no
  ticker de 100ms.
- **`goroutines-x-tomcat.md`:** "epoll por baixo, parece síncrono em cima"
  vira algo observável quando o sender fica parado num número específico de
  bytes governado pelo kernel.

---

## `Dial` (cliente) vs loop `Accept` (server)

Lados opostos da mesma conexão TCP. Papéis assimétricos.

### Tabela comparativa

| Aspecto | `Accept` (loop server) | `Dial` (cliente) |
|---------|------------------------|------------------|
| Quem chama | Servidor | Cliente |
| Pré-requisito | `net.Listen` primeiro | Nenhum |
| Estado inicial do socket | `LISTEN` (já existe, criado pelo `Listen`) | Não existe — `Dial` cria do zero |
| Syscalls escondidas | `accept()` | `socket()` + `connect()` |
| Posição no handshake | **Passivo** — espera SYN chegar, responde SYN+ACK | **Ativo** — envia o SYN inicial |
| Quem inicia | Nunca inicia. Reage. | Sempre inicia. |
| Vive em loop? | Sim — `for { Accept() }` aceita N clientes | Não — chama uma vez por conexão |
| Devolve socket novo? | Sim, a cada chamada (novo FD por cliente) | Sim, uma vez (FD único pra essa conexão) |
| Listener continua existindo? | Sim — listener intacto, vira N filhos | N/A — não existe listener |
| Porta local | Fixa (porta de escuta, ex: `:8080`) | Efêmera (kernel aloca, ex: `:51234`) |

### Esqueletos lado a lado

**Server (loop infinito, 1 listener → N conns):**
```go
listener, _ := net.Listen("tcp", ":8080")  // 1 socket LISTEN
for {
    conn, _ := listener.Accept()           // novo socket ESTABLISHED por iteração
    go handle(conn)
}
```

**Cliente (1 chamada, 1 conn):**
```go
conn, _ := net.Dial("tcp", "localhost:8080")  // 1 socket ESTABLISHED
// usa conn
```

### O handshake do ponto de vista dos dois lados

```
CLIENTE                                          SERVIDOR
                                                 net.Listen() → socket LISTEN
                                                 for { Accept() ← bloqueado }
net.Dial()
  ├─ socket()
  └─ connect() ──── SYN ──────────────────►
                  ◄────────── SYN+ACK ──── (kernel responde, app ainda em Accept)
              ──── ACK ──────────────────►
              [handshake completo]
  Dial retorna conn                              Accept retorna conn
                                                 (loop volta pra Accept de novo)
```

Os dois "se encontram" no `ACK` final. **`Dial` e `Accept` retornam quase ao mesmo tempo**, cada lado com seu próprio FD para a mesma conexão.

### Resumo mental

- `Listen` + `Accept` = "tô parado na porta, quem quiser entra".
- `Dial` = "tô indo bater na porta de alguém".

Um sem o outro, nada acontece. Server em `Accept` sem cliente fica esperando para sempre. Cliente em `Dial` sem server toma `connection refused` na hora.

### Por que `Dial` não é em loop nesta estação

Cliente normalmente quer **uma** conexão. Se quisesse N conexões para o mesmo server, seria `for { conn := Dial(...); go usar(conn) }` — mas raro.

Em `04-buffers/client.go`, o `Dial` acontece uma vez. O loop está **dentro** dessa única conexão, fazendo `Write` repetido. Loop de bytes, não loop de conexões.

---

## Onde o recebimento acontece de verdade (anatomia do `server.go`)

O recebimento é um loop de `Read`. Todo o resto é setup — e o `Sleep` que segura
o recebimento de propósito para o buffer encher.

### Sequência

```go
listener, _ := net.Listen("tcp", ":8080")   // abre socket LISTEN na porta 8080
conn, _ := listener.Accept()                 // bloqueia até cliente chegar; devolve conn ESTABLISHED
time.Sleep(15 * time.Second)                 // NÃO lê nada por 15s ← a sabotagem
// só agora drena — em pulsos (lê 4MB, pausa 1s) até EOF
```

### O ponto-chave: `Accept` ≠ `Read`

Erro comum de intuição: achar que `Accept` já recebe os dados. **Não.**

- `Accept` só completa o handshake e devolve o `conn`. Conexão pronta, nada lido.
- A partir daí, **o kernel** recebe os bytes sozinho e empilha no **receive
  buffer**, mesmo sem o app pedir.
- O app só "recebe de verdade" quando chama `Read`.

Por isso o `Sleep` enche o buffer: cliente manda, kernel guarda, app dorme.
Buffer lota → kernel anuncia `window=0` → cliente trava no `Write`. É o mesmo
elo 3-4 da [cadeia do flow control](#cadeia-do-flow-control-o-que-acontece-quando-o-cliente-congela)
descrita acima, agora visto do lado do código.

### O drain pulsado (`io.ReadFull` + pausa)

```go
buf := make([]byte, drainChunk)   // 4 MB
for {
    n, err := io.ReadFull(conn, buf)   // lê exatamente 4MB (ou erra no fim)
    total += int64(n)
    if err != nil { break }            // io.EOF ou io.ErrUnexpectedEOF = client fechou
    time.Sleep(drainPause)             // 1s ← deixa o buffer reencher antes do próximo pulso
}
```

Não usa `io.Copy(io.Discard, ...)` (que drenaria tudo em ~10ms no loopback e
mataria a observação). Em vez disso, cada `ReadFull` **esvazia o recv buffer de
uma vez** (Recv-Q despenca pra ~0 → `window` reabre → cliente destrava e empurra
mais 4MB), e a `pausa` deixa o buffer reencher antes do próximo pulso. Resultado:
Recv-Q oscila 0↔max num sawtooth visível no `ss`, e o contador do client sobe em
degraus. Os bytes lidos são jogados fora — a estação só quer **drenar** pra provar
que o buffer estava cheio, não processar.

- `io.ReadFull(conn, buf)` lê exatamente `len(buf)` ou retorna erro.
- No fim, o client já fechou (escreveu a quota de 64MB): o último `ReadFull`
  retorna `io.EOF` (se cair exato no múltiplo de 4MB) ou `io.ErrUnexpectedEOF`
  (pulso parcial). Os dois encerram o loop limpo — é o EOF que faz o server parar.

O `total` final (≈ 64 MB, a quota inteira do client) é **maior** que o
`Send-Q + Recv-Q` que o inspector mostrou parados durante o freeze (alguns MB —
num teste real congelou em ~4 MB). O número do `ss` é o que estava **retido nos
buffers no instante do bloqueio**; o `total` é **tudo que trafegou** do começo ao
fim. Não confundir os dois.

### Mapa do recebimento

```
kernel recebe bytes  ───►  receive buffer enche (app dormindo)
                                    │
                  15s depois ──► ReadFull/pausa em loop (pulsos de 4MB)
                                    │
                  cada pulso: buffer esvazia → window reabre → cliente empurra mais
```

Resumo: o recebimento real é `Read`. O `Accept` só abre a porta; o kernel é quem
segura os bytes enquanto o app não lê.

---

## O contador em goroutine + channel (anatomia do `client.go`)

O loop de escrita precisa rodar **puro**, na velocidade máxima — é ele que mede
onde o `Write` congela. Se parasse para imprimir a cada iteração, o `fmt.Printf`
lento sujaria a medida. Solução: terceirizar a impressão para uma goroutine
separada, que conversa com o loop por um channel.

### As duas goroutines e o cano entre elas

```go
totalCh := make(chan int, 1024)   // cano de ints, buffered (1024 slots)
go func() {                       // goroutine impressora
    written := 0
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case n, ok := <-totalCh:  // chegou número novo
            if !ok { return }     // channel fechado → sai (não acontece aqui)
            written = n
        case <-ticker.C:          // passou 100ms → imprime
            fmt.Printf("\rtotal escrito: %d bytes (%d KB)   ", written, written/1024)
        }
    }
}()
```

E no `main`, o loop de escrita alimenta o cano:

```go
total := 0
for {
    n, _ := conn.Write(chunk)
    total += n
    select {
    case totalCh <- total:  // tenta empurrar o total
    default:                // channel cheio? pula, segue escrevendo
    }
}
```

### Por que o `totalCh` existe

São **duas goroutines separadas**. O `main` sabe o `total` de bytes escritos; a
goroutine impressora precisa desse número para mostrar na tela. Goroutines não
compartilham variável direto sem risco de corrida de dados — o channel é o jeito
seguro de uma mandar o número para a outra.

- **Valor que trafega:** `total`, o acumulado de bytes já escritos.
- **Quem envia:** o loop de `Write` no `main` (`totalCh <- total`).
- **Quem recebe:** a goroutine, que guarda em `written` e imprime no ticker.

### Sintaxe de channel, isolada

| Forma | Significado |
|-------|-------------|
| `make(chan int, 1024)` | cria channel de `int`, buffer de 1024 (sem o número = unbuffered) |
| `ch <- x` | **envia** `x` (seta aponta pra dentro do channel) |
| `<-ch` | **recebe** valor (seta aponta pra fora) |
| `n, ok := <-ch` | recebe valor **e** flag: `ok == false` = channel fechado e vazio |
| `<-ticker.C` | recebe só pelo efeito (interessa que chegou, não o quê) |

`select` não é `switch` de valor — é `switch` de **channels prontos**. Bloqueia
até alguma case poder rodar sem travar; se duas estão prontas, escolhe aleatório.

### As duas velocidades independentes

O `select` da goroutine casa duas fontes em ritmos diferentes:

- `<-totalCh` dispara **quando chega dado** (rápido, irregular).
- `<-ticker.C` dispara **no relógio** (fixo, 100ms).

`written` é a ponte: o dado entra a qualquer hora, a impressão sai no ritmo do
ticker. Sem o ticker, imprimiria milhares de vezes por segundo.

### O envio não-bloqueante (o `default` é a chave)

No `main`, o `select` com `default` torna o envio **não-bloqueante**: se o channel
estiver cheio, cai no `default` e segue escrevendo. O loop de `Write` **nunca**
pode travar esperando o channel — senão a medição mente. Se um update for
descartado, tudo bem: a próxima volta manda um `total` ainda mais fresco.

### Nenhum channel é fechado aqui

`totalCh` nunca recebe `close()`. O loop de `Write` roda até dar erro e faz
`return` no `main` — o que derruba o **programa inteiro**, e a goroutine morre
junto. Por isso o `if !ok { return }` é código defensivo que **nunca dispara**
nesta estação: trata um channel fechado que não acontece. Fechar um channel só
importaria se o `main` continuasse vivo e quisesse sinalizar à goroutine "pode
parar" de forma limpa — não é o caso aqui.

---

## Flow control × congestion control (o que a estação deixa de fora)

Backpressure = o consumidor sinaliza "segura aí" e o produtor desacelera em vez
de acumular sem limite. No TCP, **dois** mecanismos distintos te freiam — esta
estação prova só **um**:

| | Flow control | Congestion control |
|---|---|---|
| Quem freia | o **receiver** (app lento drenando) | a **rede** (links/filas saturados) |
| Sinal | `window=0` no ACK (explícito) | perda / ECN / RTT (inferido) |
| Variável | receive window | `cwnd` |
| Esta estação | **isola e prova** | **ausente** — loopback não tem rede |

O limite real de envio é `min(receive_window, cwnd)`: os dois freios agem juntos,
o menor vence. A estação 04 zera o primeiro (`window=0` via `Sleep`) e o segundo
nunca aperta — loopback não tem latência nem perda pra congestionar. Por isso o
que você vê congelar é flow control **puro**.

**Pro código dá no mesmo:** `conn.Write` bloqueia seja por `window` fechada (flow)
ou `cwnd` apertado (congestion). Um produtor que respeita o `Write` travando
recebe os dois backpressures de graça — não precisa de rate limit próprio.

**Ver congestion control ao vivo** seria outra estação: loopback não serve, precisa
fabricar rede ruim (`tc netem delay/loss`) e ler o `cwnd` em `ss -ti`. Fora do
escopo do 04, que fica em flow control puro.

---

## Moral

`conn.Write` em Go parece síncrono mas **só realmente bloqueia quando o send
buffer enche**. Em condições normais retorna instantaneamente — é um memcpy
para o buffer do kernel. Toda a maquinaria de epoll + flow control só fica
visível quando você sabota deliberadamente o consumidor.

O que esta estação isola é **flow control**, não congestion control — distinção
e por quê na seção acima.

Ponto de engenharia: esse bloqueio **é o backpressure que você ganha de graça**.
Um produtor que respeita o `Write` travando não precisa de rate limit próprio —
o TCP já o segura no ritmo que o consumidor aguenta. É por isso que conhecer
esse mecanismo desloca o engenheiro de "adiciono retry/buffer na aplicação"
para "deixo o flow control fazer o trabalho".

Próximo passo: estados TCP no fechamento (`05-states/`).
