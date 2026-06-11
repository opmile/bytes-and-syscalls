# Estação 03 — Listener ≠ conexão

## Pergunta-âncora

Quando o servidor faz `accept()`, ele **reusa** o socket que está escutando, ou **cria** um novo?

## Hipótese antes de rodar

- Se reusasse, como atenderia múltiplos clientes?
- Se cria novo, qual o estado do socket original?

## Rodar

**Shell A — server:**
```bash
docker exec -it socket-tcp-server bash
cd /app/03-accept
go run main.go
```

Anote `Listener FD`.

**Shell B, C — clientes (no server):**
```bash
docker exec -it socket-tcp-server bash
nc localhost 8080
```

Anote `Conn FD` impresso pra cada cliente.

**Shell D — inspector:**
```bash
docker exec -it socket-tcp-inspector bash
ss -tan
```

## O que observar

### No log do server

```
Listener FD:   3

[Accept #1] novo socket criado:
  Conn FD:     7
  Listener FD: 3   <- continua o mesmo

[Accept #2] novo socket criado:
  Conn FD:     8
  Listener FD: 3   <- continua o mesmo
```

Listener FD = 3, fixo. Cada Accept retorna FD novo (7, 8, ...). FDs baixos podem variar; o **padrão** é o que importa.

### No `ss -tan`

```
State    Local Address:Port    Peer Address:Port
LISTEN   *:8080                *:*                <- listener (Conn FD 3, sem peer)
ESTAB    127.0.0.1:8080        127.0.0.1:51234   <- conn 1 (Conn FD 7)
ESTAB    127.0.0.1:8080        127.0.0.1:51235   <- conn 2 (Conn FD 8)
```

3 sockets do lado do servidor, 1 LISTEN + 2 ESTABLISHED. Mais 2 do lado dos `nc` (peers da conexão).

### Em `/proc/<pid>/fd/`

```bash
# de outro shell no server:
PID=<pid_do_server>
ls -la /proc/$PID/fd/
```

Você vê 3 entradas `socket:[<inode>]` distintas (ou mais, se aceitou mais clientes).

## Resposta esperada

`Listen()` cria **um** socket — o listener (estado `LISTEN`). Esse socket nunca transfere bytes de aplicação; ele só recebe handshakes TCP.

`Accept()`:
1. Espera handshake completo (SYN → SYN-ACK → ACK).
2. Aloca **novo** socket no kernel (com seus próprios buffers send/receive).
3. Retorna FD desse novo socket pro processo.
4. Listener continua aceitando outras conexões.

Por isso 1 servidor atende N clientes — cada um tem socket dedicado, com tupla própria, buffers próprios, estado próprio.

## Conexão com teoria

Direto do `002_socket-tcp.md`:

> "`accept()` é importante: o servidor tem um socket que *escuta* e, para cada cliente conectado, cria um socket separado."

Você acabou de imprimir essa frase em FDs.

## Por que isso importa pra Go

Quando você faz `go handleConn(conn)`, está dando uma goroutine por **socket de conexão** — não por listener. O listener é compartilhado; cada conexão é isolada. Goroutine pode bloquear em `conn.Read()` sem afetar o `Accept()` que continua rodando em outra goroutine.

## Próximo

`04-buffers/` — agora vamos ler/escrever no socket e ver os buffers do kernel agindo.
