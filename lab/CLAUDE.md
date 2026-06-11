# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A hands-on study session, not a product. Each Go file in `socket-tcp/*/` is a small program designed to surface one specific kernel/TCP concept (file descriptors, the 4-tuple, listener-vs-connection sockets, kernel send/receive buffers, TCP state machine, socket→PID mapping via inode). The user reads `GUIDE.md` for narrative, then runs the corresponding lab.

Treat changes here as pedagogy: clarity and observability of OS-level behavior beat cleverness or production-quality abstractions.

## Architecture

The repo has three layers that work together:

1. **`GUIDE.md`** — the spine. Maps theory (from `create-a-socket-in-go.md` and external notes) to each numbered station. Edit this when changing the conceptual flow.
2. **`socket-tcp/`** — Docker environment + 6 stations of runnable Go code.
3. **Each station's `README.md`** — follows a strict pattern: pergunta-âncora → hipótese → rodar → o que observar → resposta esperada → conexão com teoria. Preserve this structure when editing.

### The Docker setup is load-bearing

The whole sessão depends on `docker-compose.yml` running two services that **share a NET namespace**:

- `server` container runs the Go programs.
- `inspector` container uses `network_mode: "service:server"` — same network namespace, different PID/MNT namespaces.

Consequence: `ss`, `cat /proc/net/tcp`, `lsof -i` from the inspector see the server's sockets. But `/proc/<pid>/fd/` lookups must be done **inside the server**, because PID namespaces are not shared. The README in `06-mapping/` flags this explicitly — keep that warning.

### Multi-binary pattern in single folder

Some stations (`04-buffers/`, `05-states/`) have multiple `package main` files in the same directory. They use `//go:build ignore` so `go run <file>.go` compiles each one independently. Do not "fix" this into separate folders unless you also update the station READMEs that reference filenames.

### Why Linux-only

`/proc/net/tcp` is Linux-specific. macOS equivalents (`sysctl net.inet.tcp.pcblist`, `lsof -i`) exist but produce different output and break the parser in `06-mapping/`. Everything runs inside the container; do not add macOS fallbacks.

## Commands

**Sessão lifecycle (run from `socket-tcp/`):**

```bash
docker compose up -d --build   # subir
docker compose ps              # verificar
docker compose down            # derrubar
```

**Open shells:**

```bash
docker exec -it socket-tcp-server bash
docker exec -it socket-tcp-inspector bash
```

**Run a station** (always inside the server container, in `/app/<station>/`):

```bash
go run main.go                          # 01-fd, 02-tupla, 03-accept, 06-mapping
go run server.go                        # 04-buffers (server side)
go run client.go                        # 04-buffers (client side)
go run server_active_close.go           # 05-states scenario A
go run server_close_wait_bug.go         # 05-states scenario B
```

**Verify shared NET namespace** (sanity check the setup):

```bash
docker exec socket-tcp-server readlink /proc/self/ns/net
docker exec socket-tcp-inspector readlink /proc/self/ns/net
# Same inode = same namespace.
```

There are no tests, no linter, no build pipeline. The "verification" of any change is: does the lab still teach the concept its README claims to teach?

## When editing

- Keep code minimal. Comments only where they explain *why* (kernel semantics, why a flag is set), not *what*.
- Station READMEs are part of the pedagogy. If you change a `.go` file's behavior, update the corresponding README's "o que observar" / "resposta esperada" sections.
- `GUIDE.md` references stations by number and concept. If you add or reorder stations, update the table in `GUIDE.md` and the "Próximo" link at the bottom of each station README.
- User writes in Portuguese (Brazilian). Match the existing tone.
