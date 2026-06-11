# Estação 01 — Socket = FD no kernel

## Pergunta-âncora

Quando você chama `net.Listen("tcp", ":8080")`, o que o kernel devolve pro processo?

## Hipótese antes de rodar

(Anote sua resposta antes de ver o resultado — isto materializa o aprendizado.)

- O kernel devolve...
- Esse "algo" aparece em algum lugar visível? Onde?

## Rodar

**Shell A — server:**
```bash
docker exec -it socket-tcp-server bash
cd /app/01-fd
go run main.go
```

Anote PID e FD que o programa imprimir.

**Shell B — também no server (mesmo PID namespace, pra ver `/proc/<pid>/fd/`):**
```bash
docker exec -it socket-tcp-server bash
PID=<o que apareceu>
FD=<o que apareceu>

ls -la /proc/$PID/fd/
readlink /proc/$PID/fd/$FD
```

## O que observar

1. **`ls -la /proc/$PID/fd/`** — lista de symlinks. Você vai ver entradas como:
   ```
   lrwx------ 1 root root 64 May  8 10:00 0 -> /dev/pts/0
   lrwx------ 1 root root 64 May  8 10:00 1 -> /dev/pts/0
   lrwx------ 1 root root 64 May  8 10:00 2 -> /dev/pts/0
   lrwx------ 1 root root 64 May  8 10:00 3 -> socket:[XXXXXX]
   ```
   FDs 0/1/2 = stdin/stdout/stderr. O FD que o programa imprimiu aponta pra `socket:[<inode>]`.

2. **`readlink /proc/$PID/fd/$FD`** → `socket:[<inode>]`. Aquele inode é a chave global do socket no kernel.

3. **No inspector (`docker exec -it socket-tcp-inspector bash`):**
   ```bash
   ss -tln
   cat /proc/net/tcp | head -3
   ```
   `ss -tln` mostra `LISTEN 0 4096 *:8080`. `/proc/net/tcp` tem a linha bruta com mesmo inode.

## Resposta esperada

- `net.Listen` resultou em syscalls `socket()`, `bind()`, `listen()` no kernel.
- O kernel alocou uma `struct socket` interna (com buffers, tabela de protocolo, estado).
- Devolveu pro processo um **file descriptor** (inteiro pequeno).
- Esse FD aparece em `/proc/<pid>/fd/` como symlink pra `socket:[<inode>]`.
- O **inode** identifica unicamente esse socket no kernel — a mesma referência que aparece em `/proc/net/tcp`.

## Por que importa

"Socket é um endpoint de comunicação" deixa de ser frase abstrata. É:
- Um inteiro (FD) na sua mão.
- Um inode no kernel.
- Uma linha em `/proc/net/tcp`.
- Um symlink em `/proc/<pid>/fd/`.

Tudo a mesma coisa, vista de ângulos diferentes.

## Dica de Docker

`docker exec -it <container> bash` abre shell novo num container já rodando. `-it` = interactive + tty. Você pode abrir N shells assim no mesmo container — todos compartilham PID/MNT namespaces (vêem mesmos processos e arquivos).

## Próximo

`02-tupla/` — vamos aceitar conexões e ver a 4-tupla aparecer.
