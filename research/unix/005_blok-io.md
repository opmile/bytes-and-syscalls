# I/O bloqueante vs não bloqueante

## O modelo fundamental

Toda operação de I/O — ler de disco, enviar pacote pela rede, escrever em socket — passa pelo kernel. O processo no user space pede ao kernel via syscall (`read`, `write`, `recv`, `send`) e o kernel decide o que fazer com a thread chamadora enquanto a operação acontece. Essa decisão é o eixo da distinção bloqueante/não bloqueante.

I/O é ordens de magnitude mais lento que CPU. Um `read` em socket TCP esperando dados pode levar microssegundos a segundos; nesse intervalo, a CPU executaria milhões de instruções. O design de I/O é fundamentalmente sobre **o que fazer com a thread durante essa espera**.

## Bloqueante (blocking I/O)

A thread chama a syscall e o kernel a coloca em estado *sleeping* (não-executável) até a operação completar. O scheduler do kernel não a considera para CPU time enquanto ela estiver bloqueada. Quando os dados chegam (ou a operação falha), o kernel marca a thread como executável e ela volta a competir por CPU.

```
thread chama read(fd) → kernel: "sem dados ainda" → thread dorme
                                                        ↓
                                          dados chegam → kernel acorda thread
                                                        ↓
                                          read retorna com os bytes
```

Do ponto de vista da thread, o `read` "demorou". Do ponto de vista do kernel, a thread simplesmente não existiu durante esse intervalo — ela foi removida da run queue.

**Consequência crítica:** uma thread bloqueada não faz mais nada. Se o servidor tem uma thread por conexão e 10 mil conexões ociosas, são 10 mil threads paradas ocupando memória (stack de ~2-8 MB cada no Linux) sem progresso útil.

## Não bloqueante (non-blocking I/O)

O file descriptor é configurado com a flag `O_NONBLOCK` (via `fcntl`). Agora a syscall tem um contrato diferente: se a operação não pode completar imediatamente, ela **retorna na hora** com erro `EAGAIN` ou `EWOULDBLOCK`. A thread continua executável.

```
thread chama read(fd) → kernel: "sem dados" → retorna EAGAIN imediatamente
thread faz outra coisa, depois tenta de novo
thread chama read(fd) → kernel: "tem dados" → retorna bytes
```

Isso resolve o problema da thread parada, mas cria outro: como saber **quando** tentar de novo sem queimar CPU em loop? Aqui entram as três estratégias da pegada de rede.

---

## Network Requests (bloqueante)

Modelo clássico HTTP request/response: o cliente abre conexão, envia request, **espera** a resposta. Em Go com `http.Get`, em Java com `HttpClient.send` síncrono, em Python com `requests.get` — a thread chamadora bloqueia até a resposta chegar ou dar timeout.

```go
resp, err := http.Get("https://api.example.com/users")
// thread fica em sleep até resposta chegar
body, _ := io.ReadAll(resp.Body)
```

Por que é bloqueante por natureza? Porque o **contrato semântico** é "me devolva a resposta deste request específico". Não há nada útil para a thread fazer no meio do caminho relacionado àquela request. Em Go isso é mascarado pelo runtime — você bloqueia a goroutine, mas o netpoller libera a thread M para rodar outras goroutines, então parece não bloqueante na perspectiva do programa. Mas a goroutine em si está bloqueada.

## WebSockets & Event Streams (não bloqueante)

A conexão fica aberta e o servidor empurra mensagens quando quer. Não há "espera por resposta" no sentido request/response — a aplicação registra um handler ("quando chegar mensagem, faça X") e segue a vida.

```javascript
const ws = new WebSocket("wss://...");
ws.onmessage = (event) => { /* handler */ };
// código continua executando, handler dispara quando dados chegam
```

A thread principal nunca senta esperando uma mensagem específica. O socket é não bloqueante, e o runtime (event loop do Node, netpoller do Go) cuida de invocar o handler quando o kernel sinaliza que há dados prontos. Server-Sent Events seguem a mesma lógica unidirecional servidor→cliente.

## Polling & I/O Multiplexado (não bloqueante)

Esta é a peça do kernel que torna o resto possível. Como uma única thread monitora milhares de file descriptors sem bloquear em nenhum especificamente?

A resposta são as syscalls de multiplexação: `select`, `poll`, `epoll` (Linux), `kqueue` (BSD/macOS), `IOCP` (Windows).

```
thread → epoll_wait(lista de fds) → kernel: "esses 3 estão prontos"
thread itera nos 3 fds prontos, lê de cada um (não bloqueante, dados estão lá)
thread chama epoll_wait de novo
```

A thread bloqueia em **um único ponto** (`epoll_wait`), esperando que **qualquer** dos N fds fique pronto. Quando o kernel acorda a thread, ela já sabe exatamente quais fds têm dados disponíveis, e o `read` em cada um retorna imediatamente porque há dados.

Aqui amarra com Unix — file descriptors são a abstração que viabiliza isso. Socket TCP, pipe, arquivo, signalfd, timerfd, eventfd — tudo é fd, tudo entra no mesmo `epoll`. O kernel não distingue: "esse fd ficou legível", e pronto.

Em um socket, ao mapear a 4-tupla e o estado TCP, esses são exatamente os objetos que o kernel monitora. O netpoller do Go chama `epoll_wait` internamente — cada goroutine fazendo `conn.Read` parece bloqueante no código, mas por baixo o runtime registrou o fd no epoll e parkou a goroutine. Quando o epoll sinaliza, o runtime desparka. É por isso que Go escala para milhões de conexões com poucas threads M: o paralelismo da espera está no kernel, não nas threads.

---

## Resumindo a hierarquia

Bloqueante e não bloqueante são propriedades do **fd + syscall**, não do protocolo. HTTP poderia ser não bloqueante (e é, em clientes async). WebSocket poderia ser implementado de forma bloqueante (e é em algumas libs). A classificação que você descreveu reflete o **uso típico**:

- Network Requests modelam diálogo síncrono → naturalmente expressos como bloqueantes
- WebSockets/SSE modelam fluxo assíncrono → exigem modelo orientado a eventos
- Polling/Multiplexing é o **mecanismo do kernel** que permite o modelo orientado a eventos escalar

O ganho de não bloqueante não é "mais rápido por operação" — uma request individual não fica mais rápida. O ganho é **densidade de conexões por thread**: um event loop com epoll atende dezenas de milhares de conexões ociosas usando uma thread, enquanto o modelo bloqueante exigiria uma thread por conexão.