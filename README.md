# bytes-and-syscalls

Notas e lab sobre o que sustenta uma conexão de rede em Go: file descriptors,
buffers de kernel, threads, epoll, sockets TCP. O foco é desfazer caixas-pretas
— por que `conn.Read()` "parece bloquear" mas escala, por que `http.Client`
tem pool, por que servidores Go precisam de `go func()` no loop de `Accept`.

**→ [APRENDIZADOS.md](APRENDIZADOS.md)** — o diário técnico do estudo: estação por
estação, com a prova ao vivo no `/proc` e no `ss` de cada conceito abaixo.

## Ideias centrais do repo

### 1. Tudo passa por `fd + syscall`. Bloqueio é propriedade disso, não do protocolo.

`read`/`write`/`recv`/`send` agem sobre buffers do kernel, não direto na rede.
Bloqueia quando o buffer do lado certo está cheio (`write`) ou vazio (`read`).
A flag `O_NONBLOCK` muda o contrato da syscall para retornar `EAGAIN` em vez
de dormir. HTTP "bloqueante" e WebSocket "não bloqueante" são padrões de uso,
não propriedades de protocolo.

→ `research/unix/005_blok-io.md`, `research/network/002_socket-tcp.md`

### 2. Densidade de conexões por thread é o que muda entre modelos de servidor.

No mesmo ponto — uma thread bloqueada em `read()` de socket vazio — Tomcat,
Node e Go divergem:

- **Tomcat**: 1 thread de SO por conexão. 10k conexões ociosas = 10k threads
  presas, ~1MB stack cada.
- **Node**: 1 thread só, `epoll`/`kqueue` por baixo, callback dispara quando
  o kernel sinaliza fd pronto.
- **Go**: parece Tomcat na sintaxe (`conn.Read()` síncrono), mas o runtime
  parka a goroutine e libera a thread M. Por baixo: mesmo `epoll` que o Node,
  em modo Edge-Triggered. Scheduler do Go faz o `await` implícito.

→ `research/unix/002_threads.md`, `research/network/002_socket-tcp.md` (seção
"Comparando Maquinários")

### 3. Epoll vence poll por manter estado no kernel.

`poll` é O(N): reenvia a lista inteira de fds toda chamada, varre dos dois
lados. `epoll_wait` é O(prontos): kernel mantém a Interest List, popula a
Ready List via drivers, devolve só os fds com evento. Go usa Edge-Triggered
no netpoller — exige fd não-bloqueante e drenar até `EAGAIN`.

→ `research/unix/006_poll-epoll.md`

### 4. `go func()` no loop de Accept é correção.

Sem goroutine separada por conexão, o `Read` do primeiro cliente parka a
goroutine principal e o servidor fica surdo a novos `Accept`. O netpoller
desbloqueia a thread, mas a goroutine principal continua presa esperando
bytes daquele cliente.

→ `lab/notebook/004_concurrency.md`

### 5. Pool de conexões existe pra não pagar 1 RTT de handshake por request.

O three-way handshake (SYN, SYN-ACK, ACK) sincroniza ISNs dos dois lados e
custa 1 RTT inteiro antes do primeiro byte de payload. TLS soma mais 1-2.
`http.Client` mantém sockets em `ESTABLISHED` ociosos no pool — request novo
pega um, pula direto para `write()`. Sem pool, o gargalo "I/O bound" inclui
overhead de setup que goroutines não resolvem.

→ `research/network/004_handshake.md`, `research/network/002_socket-tcp.md`

### 6. `/proc/PID/fd` mostra que socket é arquivo de verdade.

Cada fd virou link simbólico: `0 -> /dev/pts/0` (teclado), `3 -> arquivo.txt`,
`4 -> socket:[128456]`. VFS é a camada do kernel que faz `read()` em socket e
`read()` em arquivo serem a mesma syscall. `/proc/sys/net/` expõe tuning de
TCP em tempo real (`rmem_max`, `ip_forward`, etc).

→ `research/unix/001_the-file-abstraction.md`, `research/unix/004_vfs.md`

### 7. HTTP é convenção de texto sobre stream TCP.

`net.Listen` te dá o socket cru. `conn.Read()` te dá bytes literais
(`GET / HTTP/1.1\r\nHost: ...\r\n\r\n`). Tudo entre isso e "entender que é GET
para `/`" é parser que `net/http` esconde. Implementar do zero não é produção,
mas explica por que headers terminam em `\r\n`, por que `Content-Length`
importa, por que keep-alive existe.

→ `research/network/002_socket-tcp.md` (seção "From TCP to HTTP")

## Estrutura

```
research/           # notas conceituais, sem código
├── network/        # TCP, sockets, handshake, I/O bound, glossário (NIC, RTT, kernel)
└── unix/           # file abstraction, threads, RAM, VFS, blocking I/O, poll/epoll
    └── commands/   # curl, ss, lsof, strace, tcpdump, nc, readlink

lab/                # código rodando + notebook
├── socket-tcp/  # estações numeradas: 01-fd → 06-mapping
└── notebook/         # achados do lab (docker setup, namespaces, concorrência)
```

Critério: conceito ou nota textual → `research/`. Código executável, lab,
guia → `lab/`. Ver `lab/CLAUDE.md` para regras específicas do lab
(pedagogia das estações, `//go:build ignore` multi-binary, setup Docker
com NET namespace compartilhado).

## Lab: estações de socket TCP

Cada pasta em `lab/socket-tcp/` é uma estação isolada:

- `01-fd/` — abrir socket cru, ver o fd que o kernel devolve
- `02-tupla/` — 4-tupla (IP/porta origem/destino) identificando conexão
- `03-accept/` — handshake do ponto de vista do servidor
- `04-buffers/` — provocar bloqueio em send/receive buffer
- `05-states/` — TIME_WAIT, CLOSE_WAIT, bug clássico de não fechar
- `06-mapping/` — mapear o socket Go para a entrada em `/proc`

Setup via Docker compose (NET namespace compartilhado entre containers cliente
e servidor) — vê `lab/notebook/002_docker-setup-fundamentals.md`.

## Como seguir

Pré-requisito: Docker rodando. O lab é Linux-only (depende de `/proc/net/tcp`),
então tudo roda em container — mesmo a partir de macOS/Windows.

```bash
git clone https://github.com/opmile/bytes-and-syscalls.git
cd bytes-and-syscalls/lab/socket-tcp
docker compose up -d --build   # sobe server + inspector (NET namespace compartilhado)
docker compose ps              # esperado: 2 containers Up
```

Cada estação se trabalha com **dois shells** — um no server, outro no inspector:

```bash
# Shell A — server: roda o código da estação
docker exec -it socket-tcp-server bash
cd /app/01-fd && go run main.go

# Shell B — inspector: observa o kernel ao vivo
docker exec -it socket-tcp-inspector bash
ss -tln                 # sockets em LISTEN
cat /proc/net/tcp       # tabela bruta de sockets do kernel
ls -la /proc/<pid>/fd/  # os file descriptors do processo
```

Comandos exatos por estação em [`lab/socket-tcp/README.md`](lab/socket-tcp/README.md);
o porquê de cada conceito e a ponte com a teoria em [`lab/GUIDE.md`](lab/GUIDE.md).

## O que você vai ver

Cada estação `01`→`06` materializa **uma** ideia: você escreve a hipótese, roda, e
confere no `/proc` e no `ss` o que o kernel realmente faz — não o que a doc promete.

- `01-fd` — o socket aparece como `socket:[inode]` em `/proc/<pid>/fd/`, ao lado dos
  FDs do netpoller (epoll) do Go.
- `02-tupla` — duas conexões na mesma porta local, distinguidas pela 4-tupla.
- `03-accept` — cada `accept()` cria um FD novo; o listener fica fixo em `LISTEN`.
- `04-buffers` — o `Write()` enche o send buffer e **bloqueia** quando o receiver não
  drena (backpressure ao vivo).
- `05-states` — `TIME_WAIT` saudável vs. `CLOSE_WAIT` vazando file descriptor.
- `06-mapping` — reconstruir `inode → PID` e descobrir o dono de cada socket, como o
  `ss -p` faz por baixo.
