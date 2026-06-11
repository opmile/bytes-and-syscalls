# Setup — socket-tcp

Ambiente Linux com Go + ferramentas de inspeção de rede. Dois containers compartilhando NET namespace.

## Subir o ambiente

```bash
# Da pasta socket-tcp/
docker compose up -d --build
```

`-d` = detached. `--build` = rebuilda imagem se Dockerfile mudou.

## Verificar que está de pé

```bash
docker compose ps
```

Esperado: 2 containers `Up` — `socket-tcp-server` e `socket-tcp-inspector`.

## Como você vai trabalhar

Em cada estação, você abre **dois shells** — um pro server, outro pro inspector:

**Shell A (server):**
```bash
docker exec -it socket-tcp-server bash
# dentro do container:
cd /app/01-fd
go run main.go
```

**Shell B (inspector):**
```bash
docker exec -it socket-tcp-inspector bash
# dentro do container:
ss -tln                # lista sockets em LISTEN
cat /proc/net/tcp      # tabela bruta do kernel
ls -la /proc/<pid>/fd/ # FDs do processo
```

Como os dois containers compartilham NET namespace, o inspector enxerga os sockets do server.

## Verificando o NET namespace compartilhado

```bash
# No server:
docker exec socket-tcp-server readlink /proc/self/ns/net
# net:[4026532XXX]

# No inspector:
docker exec socket-tcp-inspector readlink /proc/self/ns/net
# net:[4026532XXX]   <- MESMO inode
```

Mesmo inode = mesmo namespace. Foi isso que você leu em `00-introduction/05-underlying-tech.md` saindo do papel.

Compara com PID namespace (que NÃO é compartilhado):

```bash
docker exec socket-tcp-server readlink /proc/self/ns/pid
docker exec socket-tcp-inspector readlink /proc/self/ns/pid
# inodes diferentes — cada container tem seu próprio PID namespace
```

> **Nota importante:** como o PID namespace **não** é compartilhado, o inspector **não** enxerga os processos do server diretamente em `ps`. Mas como o NET namespace é compartilhado, ele enxerga os **sockets** (e via `/proc/net/tcp` consegue achar inodes). Para listar `/proc/<pid>/fd/` do server, faça isso de dentro do **server**, não do inspector.

## Derrubar

```bash
docker compose down
```

## Comandos úteis durante a sessão

| Comando                                         | Pra quê                                            |
|--------------------------------------------------|----------------------------------------------------|
| `ss -tln`                                        | Sockets TCP em LISTEN                              |
| `ss -tan`                                        | Todos sockets TCP (inclui ESTABLISHED, TIME_WAIT)  |
| `ss -tnp`                                        | TCP + processo dono (precisa estar no server)      |
| `cat /proc/net/tcp`                              | Tabela bruta de sockets TCP IPv4                   |
| `ls -la /proc/<pid>/fd/`                         | FDs do processo `<pid>`                            |
| `lsof -p <pid>`                                  | Tudo que o processo tem aberto                     |
| `lsof -iTCP -nP`                                 | Sockets TCP com IPs/portas resolvidos              |
| `sysctl net.ipv4.tcp_wmem`                       | Limites do send buffer                             |
| `sysctl net.ipv4.tcp_rmem`                       | Limites do receive buffer                          |