# lsof — list open files

"Em Unix, tudo é arquivo." `lsof` materializa essa frase: lista todo arquivo aberto
por qualquer processo no sistema — arquivos comuns, sockets, pipes, devices, libs
mapeadas. Existe em macOS e Linux (mesma ferramenta, leve diferença de flags).

## Mental model

Cada linha é **um file descriptor aberto** por algum processo.

```
COMMAND   PID   USER   FD   TYPE  DEVICE  SIZE/OFF  NODE       NAME
go-server 1234  mile   3u   IPv4  98765   0t0       TCP        *:8080 (LISTEN)
go-server 1234  mile   4u   IPv4  98770   0t0       TCP        127.0.0.1:8080->127.0.0.1:51234 (ESTABLISHED)
```

- `FD` — o número do file descriptor (3, 4, 5...) + modo (`u`=read/write, `r`, `w`, `cwd`, `txt`).
- `TYPE` — `REG` (arquivo regular), `DIR`, `IPv4`/`IPv6` (socket de rede), `unix` (Unix socket), `FIFO`, `CHR`.
- `NODE` — protocolo (TCP/UDP) ou inode do arquivo.
- `NAME` — caminho do arquivo, ou endereço:porta + estado se for socket.

Essa segunda linha **é o mesmo dado** que aparece em `ss` e em `/proc/net/tcp` —
três views do mesmo socket.

## Flags do dia a dia

```bash
-i              # só sockets de rede (IPv4/v6)
-i :8080        # sockets ligados a porta 8080 (qualquer processo)
-i TCP          # só TCP
-i TCP:8080     # TCP na 8080
-i @1.2.3.4     # sockets com peer/local naquele IP
-i 4            # só IPv4
-p 1234         # só do PID 1234
-c go           # só de comandos começando com "go"
-u mile         # só do usuário mile
-n              # não resolve IP/host
-P              # não resolve porta (mostra 8080 em vez de "http-alt")
+E              # mostra "endpoints" de Unix sockets (Linux) — quem está do outro lado
-r 2            # repeat — refaz a cada 2s (tipo `watch`)
```

Combinação canônica: `lsof -nP -iTCP`.

## Exemplos no contexto do go-socket

```bash
lsof -nP -iTCP:8080                       # quem segura a 8080? (estações 1-3)
lsof -nP -iTCP -sTCP:LISTEN               # todos listeners TCP da máquina
lsof -nP -iTCP -sTCP:CLOSE_WAIT           # CLOSE_WAIT no host — estação 5
lsof -p $(pgrep go-server)                # tudo que o seu servidor tem aberto
lsof -nP -iTCP -a -p $(pgrep go-server)   # `-a` = AND entre filtros
```

A última faz "TCP **E** do meu processo" — sem o `-a`, filtros de `lsof` são OR.

## Mapeando socket → processo (à mão)

Estação 6 do guia te faz fazer isto manualmente:

```bash
# achou socket interessante em /proc/net/tcp com inode 98770
sudo find /proc -lname "socket:\[98770\]" 2>/dev/null
# /proc/1234/fd/4  ← PID 1234, FD 4
```

`lsof` faz esse loop por você e ainda mostra estado, peer, comando. A estação só
te faz construir a primeira vez pra entender o que está embaixo.

## "Quem está usando este arquivo / pasta?"

Caso clássico fora do `go-socket`:

```bash
lsof /var/log/app.log                # quem segura este arquivo
lsof +D /var/lib/postgres            # qualquer FD dentro da pasta (lento)
lsof -p 1234                         # tudo do PID 1234
```

Útil para:
- "Por que não consigo desmontar este disco?" → `lsof +D /mnt/disco`.
- "Por que este log está crescendo mas não atualiza no tail?" → processo segura FD
  do log antigo após rotação (`lsof -p PID | grep deleted`). Bug clássico.
- "Vazamento de FD?" → `lsof -p PID | wc -l` crescendo ao longo do tempo.

## macOS vs Linux

`lsof` funciona em ambos com pequenas diferenças:
- macOS: `lsof -i :8080` é seu equivalente local do `ss -tlnp` do Linux (mas
  `ss` não existe em macOS).
- Linux: você tem `ss` *e* `lsof`. Use `ss` para inspecionar muitas conexões
  rápido; `lsof` quando quer o ângulo "processo → o que ele tem aberto".

## Performance

`lsof` é lento em máquinas com muitos FDs porque varre `/proc/*/fd/*` inteiro.
Em servidor cheio, prefira `ss -tlnp` para perguntas só sobre rede. `lsof` brilha
quando a pergunta é sobre **um processo específico** ou **um arquivo específico**.

## fuser — alternativa minimalista

Para a pergunta única "quem usa este arquivo/porta?":

```bash
fuser 8080/tcp        # PID(s) que seguram TCP:8080
fuser -k 8080/tcp     # mata os processos que seguram. Cuidado.
fuser /var/log/app.log
```

Menos informativo que `lsof`, mas direto ao ponto.
