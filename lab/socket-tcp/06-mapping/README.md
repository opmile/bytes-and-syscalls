# Estação 06 — Socket → processo (mapeamento via inode)

## Pergunta-âncora

Você roda `ss -tnp` e ele mostra qual processo é dono de cada socket. Como ele faz isso? Não tem mágica — é informação que está em `/proc`. Onde, e como combinar?

## Hipótese antes de rodar

- `/proc/net/tcp` tem todos os sockets, mas e o processo dono?
- `/proc/<pid>/fd/` tem os FDs do processo, mas como saber quais são socket?
- O que conecta os dois?

## A chave: inode

O **inode do socket** aparece em dois lugares:
- **`/proc/net/tcp`**: cada linha tem o inode na coluna 10.
- **`/proc/<pid>/fd/<N>`**: symlink pra `socket:[<inode>]`.

Algoritmo do `ss -p`:
1. Varre `/proc/[0-9]*/fd/*`, lê todos os symlinks, monta `inode → pid` index.
2. Lê `/proc/net/tcp`, pra cada linha pega o inode, consulta o index.
3. Imprime tudo junto.

É o que esse programa faz.

## Rodar

**Setup: use o gerador de tráfego `_traffic`.** Ele é um único processo que
segura `1 LISTEN + N cliente + N servidor` sockets vivos, todos com o mesmo PID
— a tabela enche e cada linha resolve pra um dono conhecido. Em um shell do
server:

```bash
docker exec -it socket-tcp-server bash
cd /app/_traffic && go run main.go   # imprime o PID, segura. NÃO feche.
```

**Depois, rode o mapper em outro shell do server:**
```bash
docker exec -it socket-tcp-server bash
cd /app/06-mapping && go run main.go
```

> **Importante:** rode tudo no **server**, não no inspector. O NET namespace é o
> mesmo (então `/proc/net/tcp` mostra os mesmos sockets), mas o **PID
> namespace** não é. Inspector não enxerga os PIDs do server, então o lookup
> falharia — toda linha viraria `-`.

## O que observar

Com o `_traffic` (N=3) rodando, todos os sockets são do mesmo PID — listener,
lado cliente e lado servidor das 3 conexões:

```
LOCAL                  REMOTE                 STATE        INODE    PID    COMM
127.0.0.1:9090         0.0.0.0:0              LISTEN       12345    1842   main
127.0.0.1:9090         127.0.0.1:51234        ESTABLISHED  12350    1842   main
127.0.0.1:51234        127.0.0.1:9090         ESTABLISHED  12351    1842   main
127.0.0.1:9090         127.0.0.1:51235        ESTABLISHED  12352    1842   main
127.0.0.1:51235        127.0.0.1:9090         ESTABLISHED  12353    1842   main
```

Cada conexão aparece **duas vezes** — uma por ponta (lado servidor `:9090` e
lado cliente com porta efêmera) — porque ambas as pontas vivem no mesmo
processo. É a 4-tupla da estação 02 vista dos dois lados.

## Comparar com `ss -tnp` e `lsof -i`

```bash
ss -tnp
# Exemplo:
# State  Local             Peer              Process
# LISTEN 127.0.0.1:9090    0.0.0.0:*         users:(("main",pid=1842,fd=3))
# ESTAB  127.0.0.1:9090    127.0.0.1:51234   users:(("main",pid=1842,fd=7))

lsof -iTCP -nP
# main  1842 root  3u  IPv4  12345  0t0  TCP 127.0.0.1:9090 (LISTEN)
# main  1842 root  7u  IPv4  12350  0t0  TCP 127.0.0.1:9090->127.0.0.1:51234 (ESTABLISHED)
# main  1842 root  8u  IPv4  12351  0t0  TCP 127.0.0.1:51234->127.0.0.1:9090 (ESTABLISHED)
```

A informação é idêntica. Você reimplementou o `ss -p` em ~120 linhas de Go.

## O que cada parte do código faz

| Função              | Responsabilidade                                          |
|---------------------|-----------------------------------------------------------|
| `parseAddr`         | Hex little-endian IPv4 + hex port → string `IP:port`      |
| `parseProcNetTCP`   | Lê `/proc/net/tcp`, retorna lista de `{local, remote, state, inode}` |
| `buildInodeIndex`   | Varre `/proc/<pid>/fd/`, mapeia inode → (pid, comm)       |
| `main`              | Junta os dois e imprime                                   |

## Conexão com teoria

Direto do `create-a-socker-in-go.md`:

> "O campo `inode` é a chave para mapear o socket de volta a um processo: você varre `/proc/<pid>/fd/*`, lê os symlinks, e procura `socket:[<inode>]`. É exatamente isso que o `ss -p` faz."

Você acabou de fazer.

## Limitações

- **PID namespace:** se rodar em namespace diferente do que criou os sockets, lookup falha. Por isso rodamos no server.
- **Permissões:** processos de outros usuários podem ter `/proc/<pid>/fd/` ilegível. No container rodando como root, isso não é problema.
- **Race:** processos podem nascer/morrer entre `parseProcNetTCP` e `buildInodeIndex`. Em produção, usaria snapshot atômico (não trivial).
- **IPv6:** este código só lê `/proc/net/tcp` (IPv4). Pra cobrir tudo, ler também `/proc/net/tcp6` com parsing diferente (16 bytes, endianness por palavra de 32 bits).

## Fim da sessão

Você implementou, em código rodável, todos os conceitos de:

- `foundations/002_socket-tcp.md` (teoria)
- `notebook/009_create-a-socket-go.md` (parser /proc)

Próximos passos naturais:
- Adicionar suporte a `/proc/net/tcp6`.
- Implementar parser HTTP/1.1 manual em cima de um `conn.Read`.
- TLS por cima do socket TCP.
- Comparar com `epoll` direto (via `golang.org/x/sys/unix`) — ver o que o runtime Go esconde.
