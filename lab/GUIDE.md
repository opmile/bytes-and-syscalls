# Sentindo um Socket TCP

Sessão de estudo hands-on. Materializa, em código rodável, o que você estudou em teoria sobre sockets TCP. Tudo dentro de um container Linux — bônus pedagógico: o NET namespace que você leu sobre passa a ser observável.

> **Pré-requisitos:** Docker Desktop rodando. Editor pra `cd` no projeto.

---

## Por que Docker?

Você está em macOS. Sockets TCP são abstração POSIX, mas o **`/proc/net/tcp`** que expõe a tabela de sockets do kernel é Linux-only. Em vez de adaptar pra `lsof`/`netstat` do macOS, rodamos tudo num container Linux.

Bônus: container = **NET namespace** isolado. `cat /proc/net/tcp` dentro do container mostra **só** os sockets daquele namespace. Isso é o conceito de namespace saindo do papel.

---

## Mapa da sessão

| # | Estação        | Conceito materializado                                                  |
|---|----------------|--------------------------------------------------------------------------|
| 0 | `setup`        | Imagem + compose. Network namespace compartilhado entre 2 services.     |
| 1 | `01-fd`        | Socket = file descriptor. Você vê o FD no kernel.                       |
| 2 | `02-tupla`     | 4-tupla `(IP_src, port_src, IP_dst, port_dst)`. Vê 2 conexões distintas com mesma porta destino. |
| 3 | `03-accept`    | Listener ≠ conexão. `accept()` retorna novo FD. 1 servidor, N sockets.  |
| 4 | `04-buffers`   | `write()` escreve no send buffer do kernel, não na rede. Vê bloqueio I/O. |
| 5 | `05-states`    | State machine TCP. Captura `TIME_WAIT` e `CLOSE_WAIT` ao vivo.           |
| 6 | `06-mapping`   | Parser `/proc/net/tcp` + lookup inode → PID. Igual `ss -p` faz.         |

---

## Como usar

1. **Lê uma seção do guia.** Cada estação aqui tem o "porquê" + ponte com teoria.
2. **Vai pro lab.** `cd socket-tcp/<estação>/`. README do lab tem comandos exatos.
3. **Roda. Observa. Volta pro guia.** Confirma que viu o conceito.

---

## Teoria base (do que você já estudou)

**Socket** = endpoint de comunicação. Abstração que o SO expõe pra processo enviar/receber dados pela rede. No kernel, é um **file descriptor** — número inteiro que indexa uma estrutura interna (`struct socket`) com buffers, estado TCP, etc.

**4-tupla identifica conexão TCP:**
```
(IP origem, porta origem, IP destino, porta destino)
```
Duas conexões TCP simultâneas pro mesmo servidor coexistem porque a porta de origem difere.

**Modelo cliente-servidor (POSIX):**
```
servidor                          cliente
socket()                          socket()
bind(IP, porta)
listen()
                                  connect(IP_servidor, porta)
accept()  <--- TCP handshake --->
// retorna novo socket             // conexão estabelecida
read()/write()  <--- dados --->   read()/write()
close()                           close()
```

**`accept()` é central:** servidor tem 1 socket que escuta; pra cada cliente cria um socket separado. Por isso N clientes simultâneos.

**I/O via buffers do kernel:**
- `write(socket, dados)` → copia pro **send buffer** do kernel → retorna. TCP segmenta e envia em background.
- `read(socket, buf)` → bloqueia até **receive buffer** ter dados.
- `write` bloqueia se send buffer cheio (receiver lento ou rede congestionada).
- `read` bloqueia se receive buffer vazio.

**Em Go:** `net.Listen` esconde `socket()+bind()+listen()`. `Accept()` esconde `accept()`. `conn.Write/Read` esconde `write/read`. Parece síncrono, mas o runtime usa `epoll` por baixo e troca goroutine quando bloqueia.

---

## Estação 0 — Setup

**Objetivo:** subir ambiente Linux com Go + ferramentas de rede.

**O que você vai aprender de Docker:**
- `docker compose up` com 2 services
- `network_mode: "service:X"` — compartilhar NET namespace entre containers
- `docker exec -it` — segundo shell no container rodando
- Volume mount pra editar Go no macOS, rodar no Linux

**Vai pro lab:** `socket-tcp/README.md`

Quando o setup estiver de pé, dois containers compartilham a mesma interface de rede. Um roda servidor; outro inspeciona. Mesma `lo`, mesma tabela de sockets.

---

## Estação 1 — Socket é um FD no kernel

**Pergunta-âncora:** quando eu chamo `net.Listen("tcp", ":8080")`, o que o kernel fez?

**Resposta esperada após o lab:**
- Kernel alocou uma `struct socket` interna.
- Devolveu pro processo um **file descriptor** (inteiro).
- Esse FD aparece em `/proc/<pid>/fd/<N>` como symlink pra `socket:[<inode>]`.
- O `inode` é a chave global pra identificar esse socket no kernel.

**O que rodar:** server mínimo que loga seu próprio PID e o FD. Em outra shell do container, `ls -la /proc/<pid>/fd/`.

**Conexão com teoria:** isto é o `socket()` da tabela POSIX. Antes de bind/listen, já existe um endpoint. O FD *é* o socket do ponto de vista do processo.

**Vai pro lab:** `socket-tcp/01-fd/`

---

## Estação 2 — A 4-tupla aparece

**Pergunta-âncora:** se servidor escuta em `:8080`, e dois clientes conectam ao mesmo `:8080`, como o kernel diferencia?

**Resposta esperada após o lab:**
- Cada cliente abre uma **porta de origem efêmera** distinta (ex.: 51234, 51235).
- A 4-tupla `(IP_src, porta_src, IP_dst, porta_dst)` é única por conexão.
- `cat /proc/net/tcp` mostra: 1 linha em `LISTEN` (servidor) + N linhas em `ESTABLISHED` (uma por cliente).

**O que rodar:** servidor + 2 instâncias `nc` no container inspector. Inspeciona com `ss -tn` e `cat /proc/net/tcp`.

**Conexão com teoria:** "duas conexões TCP distintas ao mesmo servidor podem existir simultaneamente porque as portas de origem diferem" — agora você vê isso impresso.

**Vai pro lab:** `socket-tcp/02-tupla/`

---

## Estação 3 — Listener ≠ conexão

**Pergunta-âncora:** `accept()` reusa o socket que escuta, ou cria outro?

**Resposta esperada após o lab:**
- `Listen` cria um socket (FD do listener).
- Cada `Accept` retorna um **novo** socket (FD diferente). O listener continua escutando.
- Por isso o servidor atende N clientes — cada um tem socket próprio.

**O que rodar:** servidor que loga `listenerFD` no startup, e `connFD` a cada accept. Compara.

**Conexão com teoria:** "o servidor tem um socket que escuta e, para cada cliente conectado, cria um socket separado".

**Vai pro lab:** `socket-tcp/03-accept/`

---

## Estação 4 — `write()` escreve no buffer, não na rede

**Pergunta-âncora:** se eu chamo `conn.Write(payload_gigante)` e o receiver não lê, o que acontece?

**Resposta esperada após o lab:**
- Primeiros bytes vão pro **send buffer do kernel** instantaneamente.
- TCP envia, mas receiver não lê → receive buffer enche → flow control freia o sender.
- Send buffer enche → próximo `Write` **bloqueia** até liberar espaço.
- O bloqueio acontece em N bytes específico, governado por `tcp_wmem`/`tcp_rmem`.

**O que rodar:** servidor que `Sleep(30s)` antes de `Read`. Cliente envia chunks crescentes, mede onde demora. `sysctl net.ipv4.tcp_wmem` mostra os limites.

**Conexão com teoria:** "você não escreve direto na rede. Escreve no buffer do kernel, e o TCP cuida do resto." Esta estação **prova** isso quantitativamente.

**Vai pro lab:** `socket-tcp/04-buffers/`

---

## Estação 5 — Estados TCP ao vivo

**Pergunta-âncora:** quem fica em `TIME_WAIT`? Por que `CLOSE_WAIT` é sinal de bug?

**Resposta esperada após o lab:**
- `TIME_WAIT` aparece no lado que **fecha primeiro** (active close). Dura ~60s. É o kernel garantindo que pacotes atrasados não contaminem nova conexão na mesma tupla.
- `CLOSE_WAIT` aparece no lado que **recebeu FIN** mas ainda **não chamou close()**. Em produção, `CLOSE_WAIT` acumulando = bug (esqueceu `defer conn.Close()`).
- Você captura ambos no mesmo `ss -tan`.

**O que rodar:** dois cenários — (a) servidor fecha primeiro, vê servidor em TIME_WAIT; (b) servidor recebe FIN mas não fecha, vê servidor em CLOSE_WAIT.

**Conexão com teoria:** state machine TCP que aparece em todo diagrama vira observável.

**Vai pro lab:** `socket-tcp/05-states/`

---

## Estação 6 — Socket → processo (mapeamento via inode)

**Pergunta-âncora:** dado `/proc/net/tcp`, como `ss -p` descobre **qual processo** dono do socket?

**Resposta esperada após o lab:**
- `/proc/net/tcp` tem coluna `inode`.
- `/proc/<pid>/fd/<N>` é symlink pra `socket:[<inode>]`.
- Pra mapear socket→processo: varre todos os `/proc/<pid>/fd/*`, lê symlink, extrai inode, casa com `/proc/net/tcp`.
- Implementa o algoritmo. Compara saída com `ss -tnp` e `lsof -i`.

**O que rodar:** programa Go que parseia `/proc/net/tcp` (versão do `create-a-socker-in-go.md`) **e** faz lookup reverso inode→PID.

**Conexão com teoria:** o que `ss -p` faz "por mágica" agora você implementou.

**Vai pro lab:** `socket-tcp/06-mapping/`

---

## Após a sessão

Você vai ter visto, em código rodável:
- FD aparecendo em `/proc/<pid>/fd/`
- 4-tupla em `/proc/net/tcp`
- listener distinto de conn
- `Write` bloqueando em quantidade previsível de bytes
- `TIME_WAIT` e `CLOSE_WAIT` no `ss`
- mapeamento socket→PID implementado à mão

Próximo passo natural: TLS handshake por cima desse socket, ou implementar parser HTTP/1.1 manual em cima do `Read`/`Write`.
