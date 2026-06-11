# Docker Setup — Fundamentos pra Sessão

Notas tomadas durante Estação 0. Cobre dois blocos:

1. **Conceitual** — Dockerfile sozinho vs Compose, anatomia do `docker-compose.yml` do lab.
2. **Hands-on (Passo 1)** — build da imagem, inspeção de layers.

## Contexto

Já tinha base teórica de Docker (`devops/docker/`) até Dockerfile. Nunca subi Compose. Esta nota é a ponte: o que o Compose faz por cima do que já conhecia, e o passo-a-passo do primeiro contato prático.

---

## Parte 1 — Conceitual

### Dockerfile sozinho — o que já sabia

Dockerfile = receita de **imagem**. Imagem = template imutável. Container = instância rodando dessa imagem.

Fluxo manual sem Compose:

```bash
docker build -t socket-lab .            # build imagem
docker run -d --name server socket-lab  # sobe container
docker exec -it server bash             # entra
docker stop server && docker rm server  # derruba
```

Funciona pra **um** container. Dor: dois containers que precisam falar entre si = muitos flags na CLI (`--network`, `--volume`, `--name`, ordem de subida). Vira shell script frágil.

### Compose — o que entra

`docker-compose.yml` = **orquestrador local**. Declara N containers + relações entre eles num único arquivo YAML. Comando `docker compose up` lê o arquivo e:

1. Builda imagens que precisam build.
2. Cria network virtual default pros containers.
3. Sobe containers na ordem certa (respeita `depends_on`).
4. Mantém vivos até `docker compose down`.

Não é mágica nova de runtime — açúcar em cima de `docker run`. Cada `service:` vira um `docker run` por baixo.

### `docker-compose.yml` do lab — linha a linha

```yaml
services:
  server:
    build: .                       # equivale a `docker build -t ... .` antes de subir
    container_name: socket-tcp-server
    volumes:
      - .:/app                     # bind mount: pasta host ./ → /app no container
    working_dir: /app              # cd /app ao entrar
    tty: true                      # aloca TTY (terminal interativo)
    stdin_open: true               # mantém stdin aberto
```

`tty + stdin_open` = equivalente a `docker run -it`. Sem isso, `CMD ["sleep", "infinity"]` ainda manteria vivo, mas `docker exec -it` ficaria estranho. Padrão pra containers de dev.

`volumes: - .:/app` = **bind mount**. Não copia. Aponta. Edita Go no VSCode → container vê instantâneo. Sem rebuild. Sem restart. Custo: container "suja" no host se gerar arquivos.

```yaml
  inspector:
    build: .                       # mesma imagem do server
    network_mode: "service:server" # AQUI mora a mágica
    depends_on:
      - server                     # sobe server primeiro
```

`network_mode: "service:server"` = "não cria NET namespace próprio pro inspector. Use o do container `server`". Equivalente Compose do flag `docker run --network container:server`.

Consequência:
- Inspector **não** declara `ports:` — não tem rede própria pra mapear.
- `localhost` no inspector = `localhost` no server.
- `ss` no inspector lista sockets do server.
- `ifconfig`/`ip addr` mostra interfaces do server.

Outros namespaces (PID, MNT, UTS, USER) seguem isolados. (Ver detalhes em `001_shared-network-vs-shared-namespace.md`.)

### Ciclo de vida — comandos do dia-a-dia

```bash
docker compose up -d --build    # builda + sobe em background
docker compose ps               # lista status (Up / Exit)
docker compose logs server      # stdout do container server
docker compose logs -f server   # follow (tipo tail -f)
docker compose exec server bash # shell no server (mesmo que `docker exec -it`)
docker compose restart server   # restart só o server
docker compose down             # para + remove containers + network
docker compose down -v          # + remove volumes (não precisa aqui, usamos bind)
```

`-d` = detached, libera terminal. Sem `-d`, containers fazem foreground e Ctrl+C derruba.

`--build` só rebuilda quando mexer no Dockerfile ou em arquivo copiado nele. Como usamos bind mount, código Go **não** precisa de rebuild — só `apk` packages do Dockerfile pediriam.

### Por que `CMD ["sleep", "infinity"]`

Três peças: o que é `CMD`, regra de morte do container, por que `sleep infinity` resolve.

#### `CMD` — processo principal do container

`CMD` no Dockerfile diz: "quando alguém rodar `docker run` nessa imagem, **execute este comando**". Define o **processo inicial** do container.

Exemplos comuns:

```dockerfile
CMD ["nginx", "-g", "daemon off;"]   # imagem nginx oficial
CMD ["node", "server.js"]            # app Node
CMD ["python", "app.py"]             # app Python
```

Quando faço `docker run nginx`, por baixo o Docker:

1. Cria container a partir da imagem.
2. Dentro do container, executa o `CMD` → vira o **PID 1** lá dentro.

#### Regra de morte: container vive enquanto PID 1 viver

PID 1 = primeiro processo do container. No Linux normal, PID 1 = `init`/`systemd`, nunca morre.

Dentro de container, PID 1 = o `CMD` que defini.

**Regra do Docker:** PID 1 termina → container morre. Imediato. Sem cerimônia.

Demonstração mental:

```dockerfile
CMD ["echo", "oi"]
```

```bash
docker run minha-imagem
# saída: oi
# container morre 10ms depois
```

PID 1 era `echo`. `echo` imprime "oi" e termina. PID 1 morreu → container `Exited (0)`.

Por isso `nginx` tem `-g "daemon off;"` — força nginx em foreground. Se rodasse em background, o processo pai sairia logo após dar fork, e o container morreria junto.

#### Nosso problema: não temos daemon

Quero usar container como **ambiente Linux** pra rodar Go manualmente:

```bash
docker exec -it server bash
go run main.go
```

`go run main.go` só roda quando peço, via `exec`. **Não** é o processo principal do container.

Se `CMD` fosse `go run main.go`:
- Container só viveria enquanto Go rodasse.
- Ctrl+C no Go → container morre.
- Não daria pra trocar de estação dentro do mesmo container.

Quero o oposto: container vivo **independente** do que rodo dentro.

#### `sleep infinity` resolve

```dockerfile
CMD ["sleep", "infinity"]
```

`sleep infinity` = comando Unix que dorme pra sempre. Nunca termina sozinho.

Consequência:
- PID 1 = `sleep infinity`. Eterno.
- Container fica `Up` indefinidamente.
- `docker exec` quantas vezes quiser, abrir shells, rodar Go, matar Go, abrir outro shell. Container não morre porque PID 1 segue dormindo.

Padrão "container como sandbox interativo".

#### Verificar na prática

```bash
docker exec socket-tcp-server ps -ef
```

Esperado (resumido):

```
UID   PID   PPID  CMD
root    1      0  sleep infinity        ← PID 1, o CMD do Dockerfile
root    X      0  bash                  ← shell aberto via docker exec
root    Y      X  ps -ef                ← o ps que acabei de rodar
```

PID 1 = `sleep infinity`. Outros processos (`bash`, `go run`, etc) são filhos criados via `docker exec`.

Mata PID 1 manualmente pra ver:

```bash
docker exec socket-tcp-server kill 1
docker compose ps
# server: Exited
```

Container morreu. Porque matei o processo eterno.

#### Quadro mental

| Cenário                              | `CMD` típico            | Por quê                                                          |
|--------------------------------------|-------------------------|------------------------------------------------------------------|
| Container roda 1 serviço (prod)      | O binário do serviço    | Serviço = PID 1. Morre = container morre = orquestrador reinicia |
| Container = sandbox interativo (lab) | `sleep infinity`        | Quero `exec` arbitrário, processos vão e voltam                  |
| Imagem base sem `CMD`                | `/bin/sh` (default)     | Vira shell ao `docker run -it`                                   |

Lab cai no segundo caso. Por isso `sleep infinity`.

### Modelo mental: dois containers, uma rede

```
┌─────────────── Linux VM (Docker Desktop) ───────────────┐
│                                                         │
│  ┌─── server ──────┐    ┌─── inspector ──────┐          │
│  │ PID ns: A       │    │ PID ns: B          │          │
│  │ MNT ns: A       │    │ MNT ns: B          │          │
│  │ NET ns: X ◄─────┼────┼─► NET ns: X (mesma)│          │
│  │ /app (bind)     │    │ /app (bind)        │          │
│  │ processos Go    │    │ ss, lsof, cat      │          │
│  └─────────────────┘    └────────────────────┘          │
│                                                         │
└─────────────────────────────────────────────────────────┘
        ▲                          ▲
        │                          │
   docker exec -it ...         docker exec -it ...
   (shell 1)                   (shell 2)
```

Mesmo NET ns = `localhost:8080` é o mesmo socket pros dois. PID ns diferente = `ps` em cada um lista só seus próprios processos.

### Onde cada conceito reaparece

| Conceito                  | Estação                                              |
|---------------------------|------------------------------------------------------|
| Bind mount vs copy        | Já — edita Go, container vê                          |
| `docker exec` workflow    | Todas estações                                       |
| Compartilhar namespace    | Já — `network_mode: "service:..."`                   |
| Isolamento PID            | 06-mapping (precisa rodar no server, não inspector)  |
| Por que Linux-only        | 01-fd em diante (`/proc/net/tcp`)                    |

---

## Parte 2 — Passo 1 hands-on: build da imagem

Pasta:

```bash
cd /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp
```

### Build

```bash
docker build -t socket-lab .
```

- `-t socket-lab` = tag (nome).
- `.` = contexto (pasta atual, onde mora Dockerfile).

Esperado: linhas tipo `=> [1/2] FROM golang:1.23-alpine`, `=> [2/2] RUN apk add ...`, no fim `=> naming to docker.io/library/socket-lab`. Primeira vez ~60s, depois cacheia.

### Conferir imagem

```bash
docker images socket-lab
```

Saída tipo:

```
REPOSITORY   TAG       IMAGE ID       CREATED         SIZE
socket-lab   latest    abc123def      30 seconds ago  ~350MB
```

### Inspecionar layers

```bash
docker history socket-lab
```

Cada linha = um layer = um comando do Dockerfile. Veria `FROM golang:1.23-alpine`, depois `RUN apk add ...`, depois `WORKDIR /app`, depois `CMD ["sleep" "infinity"]`. Cada `RUN` cria layer cacheável.

### Conceito que sai daqui

Imagem = pilha de layers imutáveis. Container = instância dessa pilha + 1 layer escrevível no topo.

---

---

## Parte 3 — Passo 2 hands-on: subir 1 container manual

Sem Compose. CLI pura. Ver o que `docker run` faz, depois apreciar o que Compose abstrai.

### Subir

```bash
docker run -d \
  --name socket-lab-test \
  -v /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp:/app \
  -w /app \
  socket-lab
```

Decompondo cada flag:

| Flag             | O que faz                                                  | Equivalente no Compose            |
|------------------|------------------------------------------------------------|-----------------------------------|
| `-d`             | Detached. Libera terminal, container roda em background.   | `docker compose up -d`            |
| `--name X`       | Nome do container. Sem isso, Docker gera tipo `quirky_cat`.| `container_name: X`               |
| `-v HOST:CONT`   | Bind mount: pasta do host → caminho no container.          | `volumes: - .:/app`               |
| `-w /app`        | Working dir inicial dentro do container.                   | `working_dir: /app`               |
| `socket-lab`     | Nome da imagem (a que buildei no Passo 1).                 | `build: .` (referencia Dockerfile)|

Saída: hash gigante (container ID) tipo `7f2a9c1d8e...`.

> Por que caminho absoluto no `-v`? Docker CLI exige path absoluto. Compose aceita relativo porque resolve a partir da pasta do `docker-compose.yml`.

### Verificar vivo

```bash
docker ps
```

Esperado:

```
CONTAINER ID   IMAGE        COMMAND             STATUS         NAMES
7f2a9c1d8e..   socket-lab   "sleep infinity"    Up 5 seconds   socket-lab-test
```

Coluna `COMMAND` = `"sleep infinity"`. PID 1 lá dentro.

`docker ps -a` mostra todos (inclusive parados).

### Entrar

```bash
docker exec -it socket-lab-test bash
```

`exec` = roda comando **novo** num container já vivo. **Não** é "entrar no PID 1". Spawn de processo filho dentro do namespace do container.

`-i` = interactive (stdin aberto). `-t` = TTY. Junto = shell utilizável.

Confere dentro:

```bash
pwd     # /app  ← porque -w /app
ls      # arquivos do lab visíveis via bind mount
```

### Bind mount é ponteiro, não cópia

Cria arquivo dentro:

```bash
echo "vim do container" > /app/teste-bind.txt
```

Outro terminal no macOS:

```bash
cat /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp/teste-bind.txt
# vim do container
```

Apaga de qualquer lado — some dos dois. Bind mount = ponteiro.

### PID 1 visto na prática

Dentro do container:

```bash
ps -ef
```

Esperado:

```
UID  PID  PPID  CMD
root   1     0  sleep infinity        ← PID 1, o CMD do Dockerfile
root   X     0  bash                  ← shell aberto via docker exec, PPID=0
root   Y     X  ps -ef                ← filho do bash
```

PID namespace isolado: só vejo processos **deste** container. No macOS, `ps aux | wc -l` mostraria 500+ processos. Dois mundos.

### Sair sem matar

```bash
exit
```

Container **continua vivo** (PID 1 ainda é `sleep`). `docker exec` repetível, cada chamada = novo processo, novo shell.

### Derrubar

```bash
docker stop socket-lab-test   # vira Exited, ainda existe
docker rm socket-lab-test     # remove de vez
```

Atalho: `docker rm -f socket-lab-test` (força stop + rm em um comando).

### Mapeamento Manual ↔ Compose

| Manual                                       | Compose equivalente                |
|----------------------------------------------|------------------------------------|
| `docker run -d --name X -v ... -w ... image` | `docker compose up -d`             |
| `docker exec -it X bash`                     | `docker compose exec X bash`       |
| `docker stop X && docker rm X`               | `docker compose down`              |

Compose lê o YAML e dispara essas mesmas chamadas, mais lógica de network, ordem de dependência, etc.

### O que materializou aqui

- Bind mount: editei de um lado, vi do outro.
- PID 1 = `sleep infinity`: visto em `ps -ef`.
- `docker exec`: processo novo em namespace existente.

---

## Parte 4 — Passo 3 hands-on: dois containers manuais, NET namespace compartilhado

Coração do lab. Materializa o `network_mode: "service:server"` do Compose. Prova com inodes de namespace que dois containers compartilham rede.

### Sobe o "server"

```bash
docker run -d \
  --name lab-server \
  -v /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp:/app \
  -w /app \
  socket-lab
```

Igual ao Passo 2. NET namespace **próprio** (Docker cria automaticamente).

### Sobe o "inspector" entrando no NET namespace do server

```bash
docker run -d \
  --name lab-inspector \
  --network container:lab-server \
  -v /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp:/app \
  -w /app \
  socket-lab
```

**Flag novo:** `--network container:lab-server`.

Tradução: "não cria NET namespace pra esse container. Usa o do `lab-server`".

> Ausência de `-p` / `--publish`. Não pode publicar portas em container que não tem rede própria. Mesma restrição do Compose: service `inspector` não declara `ports:`.

### Provar que NET namespace é o mesmo

Cada namespace Linux tem inode único. Visível em `/proc/self/ns/<tipo>`. Mesmo inode = mesmo namespace.

```bash
docker exec lab-server readlink /proc/self/ns/net
docker exec lab-inspector readlink /proc/self/ns/net
# net:[4026532XXX] nos dois  ← MESMO número
```

Inodes idênticos. Não é "duas redes conectadas". É **uma rede, dois containers**.

### Provar que outros namespaces são diferentes

NET é o **único** compartilhado. PID, MNT, UTS, USER seguem isolados.

```bash
docker exec lab-server readlink /proc/self/ns/pid
docker exec lab-inspector readlink /proc/self/ns/pid
# inodes diferentes
```

| Namespace | Isolamento entre lab-server e lab-inspector |
|-----------|---------------------------------------------|
| `net`     | **Compartilhado** (mesmo inode)             |
| `pid`     | Isolado                                     |
| `mnt`     | Isolado                                     |
| `uts`     | Isolado                                     |
| `user`    | Isolado                                     |

### Ver efeito prático: socket criado em um, visível no outro

Dois shells lado a lado.

**Shell A — server:**
```bash
docker exec -it lab-server bash
nc -l -p 9999    # netcat escutando na porta 9999, pendurado
```

**Shell B — inspector:**
```bash
docker exec -it lab-inspector bash
ss -tln | grep 9999
# LISTEN  0  1  0.0.0.0:9999  0.0.0.0:*
```

**Inspector enxerga o socket que server criou.** Sem rede no meio. Mesma tabela do kernel — mesmo NET namespace.

`/proc/net/tcp` direto também mostra:

```bash
cat /proc/net/tcp | grep -i 270F   # 270F hex = 9999 decimal
```

Fechamento da prova: `localhost` no inspector resolve pro mesmo socket:

```bash
# no inspector:
echo "oi do inspector" | nc -q 0 localhost 9999
```

Shell A vê chegar `oi do inspector`. `localhost:9999` no inspector é literalmente o mesmo `localhost:9999` do server. Loopback único compartilhado.

### Comparar PID namespace isolado

Inspector roda:

```bash
ps -ef
# vê só processos do inspector (bash, ps).
# NÃO vê o nc do server.
```

PID namespace separado = tabela de processos separada. Por isso `06-mapping` precisa rodar dentro do **server** (precisa do `/proc/<pid>/fd/` dos processos do server).

Mas NET namespace compartilhado = `/proc/net/tcp` é o mesmo. Por isso inspector consegue inspecionar sockets.

### Derrubar

```bash
docker rm -f lab-inspector lab-server
```

Mata inspector primeiro. Se matar server antes, inspector fica órfão (NET ns destruído sob ele).

### Conceitos que materializaram aqui

| Teoria                                         | Onde vi na prática                                              |
|------------------------------------------------|-----------------------------------------------------------------|
| Linux namespace = mecanismo de isolamento      | `readlink /proc/self/ns/net` retornando inode                   |
| Cada container = conjunto de namespaces        | Vários `ns/<tipo>` independentes em `/proc/self/ns/`            |
| Compartilhar namespace = entrar no mesmo inode | `--network container:X` colocou inspector no NET ns do server   |
| Loopback é por NET namespace                   | `localhost:9999` no inspector = mesmo socket do server          |
| `/proc/net/tcp` é por NET namespace            | Inspector lê e enxerga sockets do server                        |
| `/proc/<pid>` é por PID namespace              | Inspector **não** vê `nc` do server em `ps`                     |

### Mapeamento manual ↔ Compose

| Manual                                                | Compose                            |
|-------------------------------------------------------|------------------------------------|
| `docker run --name lab-server ... socket-lab`         | service `server` no YAML           |
| `docker run --network container:lab-server ...`       | `network_mode: "service:server"`   |
| Subir server primeiro, inspector depois               | `depends_on: [server]`             |

---

## Q&A — uma imagem serve pros dois containers?

Sim. Uma imagem, dois containers.

**Imagem** = o que está instalado (Go, `ss`, `lsof`, `bash`, etc).
**Container** = instância rodando + papel que dou a ela.

Imagem não decide papel. Papel vem de **como** uso o container depois.

No lab:
- `lab-server`: entro e rodo `go run main.go`. Papel = servidor Go.
- `lab-inspector`: entro e rodo `ss`, `cat /proc/net/tcp`, `lsof`. Papel = inspetor.

Mesma imagem suporta os dois porque o Dockerfile já instalou as ferramentas dos dois lados:

```dockerfile
FROM golang:1.23-alpine                                # Go (pro server)
RUN apk add iproute2 lsof procps busybox-extras bash   # ferramentas (pro inspector)
```

### Por que escolha pedagógica

Em prod, faria sentido duas imagens — uma magra com só binário Go (sem `ss`), outra com tooling de debug. Boa prática: imagem mínima por papel.

Aqui é estudo. Vantagens da imagem única:
- Um `docker build` só, cacheia rápido.
- Inspector tem Go também (útil mentalmente, embora `06-mapping` exija PID ns do server).
- Server tem `ss` — posso inspecionar de dentro do server sem trocar de container.

Sobreposição de ferramentas custa MB de imagem, ganha simplicidade pedagógica.

### Prova rápida

```bash
docker exec lab-server which go ss
docker exec lab-inspector which go ss
# mesmos binários nos dois
```

Papel ≠ imagem.

---

## Parte 5 — Passo 4 hands-on: tudo de novo, agora com Compose

Tudo que fiz à mão no Passo 3 está declarado no `docker-compose.yml` do lab. Um comando, mesmo estado final.

### Ver YAML resolvido antes de subir

```bash
cd /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp
docker compose config
```

`config` = parseia o YAML, resolve variáveis, expande paths. Útil pra debug. Vê paths absolutos do bind mount expandidos.

### Subir stack

```bash
docker compose up -d --build
```

Saída:

```
[+] Running 2/2
 ✔ Container socket-tcp-server     Started
 ✔ Container socket-tcp-inspector  Started
```

Ordem: **server primeiro, inspector depois**. Por causa de `depends_on: [server]`. Mesma ordem do Passo 3.

### Verificar

```bash
docker compose ps
```

Esperado:

```
NAME                    IMAGE              STATUS
socket-tcp-server       socket-tcp-server  Up X seconds
socket-tcp-inspector    socket-tcp-server  Up X seconds
```

Atenção:
1. **Nome dos containers** vem de `container_name:` no YAML (sem isso, sufixo `-1`).
2. **Imagem** = `socket-tcp-server` (nome do diretório + service).

### Replicar testes do Passo 3

NET ns compartilhado:
```bash
docker exec socket-tcp-server readlink /proc/self/ns/net
docker exec socket-tcp-inspector readlink /proc/self/ns/net
# mesmos inodes
```

PID ns isolado:
```bash
docker exec socket-tcp-server readlink /proc/self/ns/pid
docker exec socket-tcp-inspector readlink /proc/self/ns/pid
# inodes diferentes
```

Teste de socket cruzado: igual ao Passo 3, comportamento idêntico.

**Comportamento idêntico ao Passo 3.** Mudou só ergonomia.

### Comandos do dia-a-dia

| Manual (Passo 3)                          | Compose                                    |
|-------------------------------------------|--------------------------------------------|
| `docker run -d --name server ...`         | `docker compose up -d`                     |
| `docker exec -it server bash`             | `docker compose exec server bash`          |
| `docker logs server`                      | `docker compose logs server`               |
| `docker logs -f server`                   | `docker compose logs -f server`            |
| `docker stop server inspector`            | `docker compose stop`                      |
| `docker rm -f server inspector`           | `docker compose down`                      |
| (sem comando — listar e filtrar manual)   | `docker compose ps`                        |

Detalhe `exec`: tanto `docker exec -it socket-tcp-server bash` quanto `docker compose exec server bash` funcionam. Diferença:
- `docker exec` usa nome do **container** (`socket-tcp-server`).
- `docker compose exec` usa nome do **service** (`server`).

### Logs em tempo real (gotcha)

PID 1 = `sleep infinity` = log vazio. Quando rodo `go run main.go` via `exec`, stdout fica no terminal do exec, **não** no log do container.

Pra logar processos rodados via exec, redireciona explícito:

```bash
docker compose exec server bash -c "go run /app/01-fd/main.go" 2>&1 | tee log.txt
```

(Não preciso disso nas estações — vou rodar interativo. Só pra saber que `docker logs` não é mágico.)

### Derrubar

```bash
docker compose down
```

Para + remove containers + remove network virtual criada. Não toca a imagem, nem volumes bind (sem volume nomeado aqui).

Pra apagar imagem também: `docker compose down --rmi local`.

### O que Compose poupou vs Passo 3

| Fricção no manual                              | Como Compose resolve                          |
|------------------------------------------------|-----------------------------------------------|
| Caminho absoluto no `-v`                       | YAML aceita relativo, resolve a partir do dir |
| Lembrar ordem de subida                        | `depends_on:`                                 |
| Lembrar flag `--network container:X`           | `network_mode: "service:X"`                   |
| Buildar manualmente antes de rodar             | `--build` faz no `up`                         |
| Listar containers e filtrar                    | `docker compose ps` (escopo do projeto)       |
| Derrubar dois containers + uma network         | `docker compose down`                         |

Não é poder novo. **Declaração** em vez de **imperativo**. Descrevo estado desejado, Compose calcula diff.

---

---

## Parte 6 — Passo 5 hands-on: subir stack final do lab

Sem novidade conceitual. Checkpoint operacional + verificações de sanidade pra arrancar pro `01-fd/`.

### Garantir estado limpo

```bash
cd /Users/milenaoliveirapenhalves/code/go/bytes-and-syscalls/lab/socket-tcp
docker compose ps
```

Se aparecer algo Up de sessões anteriores, derruba:

```bash
docker compose down
```

Limpar containers órfãos manuais (Passos 2/3):

```bash
docker ps -a | grep -E 'lab-server|lab-inspector|socket-lab-test'
docker rm -f lab-server lab-inspector socket-lab-test 2>/dev/null
```

### Subir

```bash
docker compose up -d --build
```

Sem rebuild custoso — imagem cacheada. Rápido.

Esperado:
```
 ✔ Container socket-tcp-server     Started
 ✔ Container socket-tcp-inspector  Started
```

### Sanidade de namespace

```bash
docker exec socket-tcp-server readlink /proc/self/ns/net
docker exec socket-tcp-inspector readlink /proc/self/ns/net
# mesmos inodes → NET ns compartilhado ✓

docker exec socket-tcp-server readlink /proc/self/ns/pid
docker exec socket-tcp-inspector readlink /proc/self/ns/pid
# inodes diferentes → PID ns isolado ✓
```

(Detalhes de `/proc/self/ns/`, inodes e `readlink` → ver `003_proc-namespaces-inodes-readlink.md`.)

### Sanidade de bind mount

```bash
docker exec socket-tcp-server ls /app/socket-tcp
# 01-fd  02-tupla  03-accept  04-buffers  05-states  06-mapping  ...
```

Mount ligado. Edita Go no VSCode → container vê na hora.

### Sanidade de Go

```bash
docker exec socket-tcp-server go version
# go version go1.23.x linux/amd64
```

### Workflow das estações

Cada estação pede 2 shells. Padrão:

**Terminal A — server:**
```bash
docker exec -it socket-tcp-server bash
cd /app/<estacao>
```

**Terminal B — inspector:**
```bash
docker exec -it socket-tcp-inspector bash
```

A roda Go, B roda `ss`/`cat /proc/net/tcp`/`lsof`.

`05-states` e `06-mapping` ocasionalmente pedem terceiro shell ou que rode dentro do **server** (não inspector) — README de cada estação avisa.

### Fim de sessão

```bash
docker compose down
```

Não perde nada — código mora no macOS via bind mount. Imagem fica cacheada.

### Checklist final

- [x] `docker compose ps` mostra 2 containers Up
- [x] NET ns inodes batem
- [x] PID ns inodes não batem
- [x] `/app/socket-tcp` lista os diretórios das estações
- [x] `go version` responde
- [x] Dois terminais abertos (A=server, B=inspector)

Checkpoint fechado. Pronto pro `01-fd/`.
