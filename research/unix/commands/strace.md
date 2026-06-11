# strace — tracing de syscalls

`strace` mostra **toda chamada de sistema** que um processo faz, em tempo real.
Para o currículo do `go-socket`, é a ferramenta que prova literalmente que sua
goroutine chamou `socket()`, depois `bind()`, depois `listen()`, depois `accept()`.

> Linux-only. Equivalente em macOS é `dtruss` (precisa desativar SIP) ou
> `dtrace` direto. No container do lab, instala com `apt install strace`.

## Mental model

Todo programa em Linux precisa do kernel para qualquer operação além de cálculo
puro: abrir arquivo, ler de socket, alocar memória grande, dormir, criar thread.
Essas operações são **syscalls**. `strace` se prende a um processo (via `ptrace`)
e imprime cada uma — nome, argumentos, valor de retorno.

```
socket(AF_INET, SOCK_STREAM|SOCK_CLOEXEC|SOCK_NONBLOCK, IPPROTO_IP) = 3
setsockopt(3, SOL_SOCKET, SO_REUSEADDR, [1], 4) = 0
bind(3, {sa_family=AF_INET, sin_port=htons(8080), sin_addr=inet_addr("0.0.0.0")}, 16) = 0
listen(3, 4096) = 0
accept4(3, {sa_family=AF_INET, sin_port=htons(51234), sin_addr=inet_addr("127.0.0.1")}, [16], SOCK_CLOEXEC|SOCK_NONBLOCK) = 4
```

Isso é o que `net.Listen("tcp", ":8080")` mais um `Accept()` viram em Go. O `= 3`
e `= 4` são os file descriptors que voltam — exatamente o que estação 1 do guia
te faz observar em `/proc/<pid>/fd/`.

## Formas de invocar

```bash
strace ./meu-prog                  # roda o programa sob strace desde o início
strace -p 1234                     # se prende a processo já rodando (precisa permissão)
strace -f ./meu-prog               # segue fork() — necessário pra programas multi-thread/multi-proc
strace -ff -o trace ./meu-prog     # um arquivo por thread/pid
```

Para Go, `-f` é **obrigatório**: o runtime cria threads (`clone()`). Sem `-f`
você só vê syscalls da main thread e perde tudo das goroutines paralelas.

## Filtros — sobrevivência

`strace` sem filtro produz milhares de linhas por segundo. Filtra cedo:

```bash
-e trace=network              # só socket, bind, accept, connect, send, recv, etc.
-e trace=%file                # qualquer syscall que recebe path
-e trace=read,write           # explícito
-e trace=!futex,clock_gettime # exclui ruído (Go faz muito futex)
-e signal=none                # esconde sinais
```

Conjuntos prontos úteis:
- `%file` — toda manipulação de arquivo por path.
- `%network` — toda syscall de rede.
- `%process` — fork, exec, exit, wait.
- `%signal`, `%ipc`, `%memory`.

## Flags de saída

```bash
-t / -tt / -ttt    # timestamps (segundos / microssegundos / epoch)
-T                 # mostra tempo gasto em cada syscall
-c                 # NÃO printa, só faz contagem agregada no final
-s 200             # mostra até 200 bytes de strings (default trunca em 32)
-y                 # mostra path/peer ao lado de cada FD ("3<TCP:[127.0.0.1:8080]>")
-yy                # `-y` + detalhes (protocolo, IP)
-o trace.log       # saída pra arquivo
```

`-yy -f -e trace=%network` é a combinação que torna trace de rede legível.

## Exemplos no contexto do go-socket

```bash
# estação 1 — ver socket() bind() listen() acontecerem
strace -f -e trace=%network -yy ./server

# estação 3 — ver accept() retornar FDs novos a cada conexão
strace -p $(pgrep server) -f -e trace=accept4 -yy

# estação 4 — ver write() bloqueando quando send buffer enche
strace -p $(pgrep client) -f -e trace=write -T -y
# o `-T` mostra duração — write normal = µs, write bloqueado = segundos

# estação 5 — capturar o close() e ver quem fecha primeiro
strace -p $(pgrep server) -f -e trace=close,shutdown -yy
```

## Lendo a saída

Cada linha é `syscall(args) = retorno`.

- Retorno positivo ou 0: sucesso. Para `read`/`write`, é o número de bytes.
- Retorno `-1` vem com `errno`: `read(...) = -1 EAGAIN (Resource temporarily unavailable)`.
- `EAGAIN` em socket não-bloqueante é o sinal de "nada pra ler agora" — é o que
  o runtime Go traduz em "parqueia goroutine, registra no epoll, troca pra outra".

Estado interno do Go que você não vê do lado da linguagem aparece aqui:
- `epoll_create1` — Go cria seu epoll fd na inicialização.
- `epoll_ctl` — quando adiciona/remove socket do interesse.
- `epoll_wait` — netpoller dormindo esperando I/O.

Cruzar `strace -e %network` com a leitura `003_poll-epoll.md` materializa o
netpoller do runtime.

## Custo e cuidados

- `strace` **desacelera o processo** brutalmente — até 10-100x em workloads
  syscall-heavy. Não rode em produção sob carga.
- Cada syscall vira context switch extra. Comportamento sob `strace` ≠
  comportamento sem.
- Em servidor de produção real, prefira `bpftrace`/`perf`/`bcc-tools` — usam
  eBPF, custo muito menor.

## Variantes e alternativas

- **`ltrace`** — mesma ideia, mas para chamadas de biblioteca (libc) em vez de
  syscalls. Útil em programas C; em Go praticamente irrelevante (Go não usa
  libc na maioria das syscalls).
- **`bpftrace`** — traça syscalls com eBPF, baixo overhead, sintaxe própria
  (`bpftrace -e 'tracepoint:syscalls:sys_enter_accept4 { printf("%s\n", comm); }'`).
  Próxima parada quando `strace` ficar lento demais.
- **`perf trace`** — alternativa moderna do kernel, parecido com strace mas com
  amostragem.

Para estudo e debug local, `strace` segue insubstituível por ser óbvio e estar
em qualquer Linux.
