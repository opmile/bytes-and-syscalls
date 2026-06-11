# Estação 05 — Estados TCP ao vivo

## Pergunta-âncora

Por que existe `TIME_WAIT`? E `CLOSE_WAIT`? Quem fica em qual? Por que profissionais de ops/dev se preocupam tanto com `CLOSE_WAIT` acumulando?

## Hipótese antes de rodar

- TIME_WAIT: aparece em qual lado da conexão?
- CLOSE_WAIT: o que diferencia da CLOSE normal?
- Qual estado é "saudável" e qual é sinal de bug?

## Diagrama TCP de fechamento (ref)

```
Lado A (active close)              Lado B (passive close)
       |                                  |
       |  FIN                             |
       |--------------------------------->|
       |                                  | CLOSE_WAIT
       |                       ACK        |
       |<---------------------------------|
       |                                  |
       |                       FIN        |
       |<---------------------------------|  (depois que B chamar close)
       |                                  | LAST_ACK
       |  ACK                             |
       |--------------------------------->|
TIME_WAIT (espera 2*MSL ~60s)             CLOSED
       |
CLOSED
```

Quem fecha primeiro = `TIME_WAIT`. Quem recebe FIN sem ter fechado seu lado = `CLOSE_WAIT`.

---

## Cenário A — TIME_WAIT no servidor

### Rodar

**Shell A — server (fecha primeiro):**
```bash
docker exec -it socket-tcp-server bash
cd /app/05-states
go run server_active_close.go
```

**Shell B — cliente:**
```bash
docker exec -it socket-tcp-server bash
echo "oi" | nc localhost 8080
# nc fica vivo até o servidor fechar; depois ele sai
```

**Shell C — inspector (rapidamente, durante a janela TIME_WAIT):**
```bash
docker exec -it socket-tcp-inspector bash
ss -tan | grep 8080
```

### O que observar

```
State       Recv-Q Send-Q Local Address:Port  Peer Address:Port
LISTEN      0      4096   *:8080              *:*
TIME-WAIT   0      0      127.0.0.1:8080      127.0.0.1:51234
```

Lado servidor entra em `TIME-WAIT`. Permanece ~60s e desaparece. Você vai ver sumir se ficar observando.

### Por que TIME_WAIT existe

1. **Garantir que o ACK final chegue.** Se o ACK do FIN do peer se perder, o peer reenvia FIN. Se o socket já tivesse sumido, ninguém respondia.
2. **Evitar pacotes "zumbis" da conexão antiga contaminarem uma conexão nova com a mesma 4-tupla.** Por isso a porta fica "presa" pelo tempo de TIME_WAIT.

A duração `2 * MSL` (Maximum Segment Lifetime) costuma ser ~60s no Linux.

---

## Cenário B — CLOSE_WAIT (bug)

### Rodar

**Reset entre cenários:**
```bash
# pare o server do cenário A com Ctrl+C antes de seguir
```

**Shell A — server bugado (não fecha):**
```bash
docker exec -it socket-tcp-server bash
cd /app/05-states
go run server_close_wait_bug.go
```

**Shell B — cliente que fecha primeiro:**
```bash
docker exec -it socket-tcp-server bash
echo "oi" | nc -w1 localhost 8080
# -w1: nc do BusyBox espera 1s no final e fecha (manda EOF/FIN).
# BusyBox não tem -q (flag do GNU netcat).
```

**Shell C — inspector:**
```bash
docker exec -it socket-tcp-inspector bash
ss -tan | grep 8080
```

### O que observar

```
State       Recv-Q Send-Q Local Address:Port  Peer Address:Port
LISTEN      0      4096   *:8080              *:*
CLOSE-WAIT  XX     0      127.0.0.1:8080      127.0.0.1:51234
```

Lado servidor em `CLOSE-WAIT`. **Não desaparece** — fica até o processo do server morrer. Fonte clássica de vazamento de socket em produção (esquecer `defer conn.Close()` em handler).

### Por que é bug

`CLOSE-WAIT` significa: "recebi FIN do peer, mas eu (aplicação) ainda não chamei `close()` no meu lado". O kernel está esperando você. Em produção, código que aceita conexões mas não fecha em todos os branches acumula `CLOSE-WAIT`. Eventualmente, esgota FDs (ulimit) ou portas efêmeras.

---

## Comparativo dos estados

```bash
# Durante o cenário A:
ss -tan | grep 8080
# TIME-WAIT  -> some sozinho

# Durante o cenário B:
ss -tan | grep 8080
# CLOSE-WAIT -> persiste, é bug

# /proc/net/tcp mostra o mesmo em hex:
cat /proc/net/tcp | grep -i 1F90  # 1F90 = porta 8080 em hex
# Coluna 4 (state):
#   06 = TIME_WAIT
#   08 = CLOSE_WAIT
#   01 = ESTABLISHED
#   0A = LISTEN
```

Os hexes da coluna 4 batem com a tabela do `create-a-socker-in-go.md`.

---

## Resposta esperada

| Estado      | Quem entra                            | Saudável?          | Duração       |
|-------------|---------------------------------------|--------------------|---------------|
| ESTABLISHED | ambos lados, conexão ativa            | sim                | enquanto usa  |
| TIME_WAIT   | lado que fechou primeiro (active)     | sim, esperado      | ~60s          |
| CLOSE_WAIT  | lado que recebeu FIN e não fechou     | **bug** se acumula | até processo morrer |
| LISTEN      | servidor escutando                    | sim                | enquanto roda |

## Conexão com teoria

Os hexes em `tcpStates` do `create-a-socker-in-go.md` agora têm rosto. `06`/`08`/`0A`/`01` deixam de ser tabela e viram observação direta.

## Próximo

`06-mapping/` — última estação. Vamos implementar o que `ss -p` faz: dado um socket no `/proc/net/tcp`, descobrir qual processo é dono.
