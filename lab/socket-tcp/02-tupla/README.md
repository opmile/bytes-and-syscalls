# Estação 02 — A 4-tupla

## Pergunta-âncora

Servidor escuta em `:8080`. Dois clientes conectam ao mesmo `:8080`. Como o kernel diferencia as duas conexões? Por que não há "colisão"?

## Hipótese antes de rodar

- Como o kernel sabe pra qual conexão entregar um pacote que chega?
- Se ambos clientes usam IP `127.0.0.1`, o que difere?

## Rodar

**Shell A — server:**
```bash
docker exec -it socket-tcp-server bash
cd /app/02-tupla
go run main.go
```

**Shell B — cliente 1 (no server):**
```bash
docker exec -it socket-tcp-server bash
nc localhost 8080
# fica vivo aguardando input
```

**Shell C — cliente 2 (no server):**
```bash
docker exec -it socket-tcp-server bash
nc localhost 8080
# fica vivo aguardando input
```

**Shell D — inspector:**
```bash
docker exec -it socket-tcp-inspector bash
ss -tn
cat /proc/net/tcp
```

## O que observar

### No log do server

```
[conn 1] tupla:
  Local  (server side): 127.0.0.1:8080
  Remote (client side): 127.0.0.1:51234
[conn 2] tupla:
  Local  (server side): 127.0.0.1:8080
  Remote (client side): 127.0.0.1:51235
```

`Local` é igual nas duas (mesma porta de escuta). `Remote` difere nas portas — **portas efêmeras** alocadas pelo kernel pra cada cliente.

### No `ss -tn` do inspector

```
State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port
ESTAB   0       0       127.0.0.1:8080      127.0.0.1:51234
ESTAB   0       0       127.0.0.1:8080      127.0.0.1:51235
ESTAB   0       0       127.0.0.1:51234     127.0.0.1:8080
ESTAB   0       0       127.0.0.1:51235     127.0.0.1:8080
```

> Atenção: você vê **4 linhas**, não 2. Por quê? Cada conexão tem dois lados, e ambos os lados estão **dentro do mesmo container** (loopback). Cliente e servidor são dois sockets distintos, cada um com seu FD, cada um aparecendo na tabela. Em conexões reais entre máquinas, você só vê seu lado.

### No `/proc/net/tcp`

Mesma informação em hex. `0100007F:1F90` = `127.0.0.1:8080` (portas em hex, IP little-endian invertido).

## Resposta esperada

A 4-tupla `(IP_src, porta_src, IP_dst, porta_dst)` é **única por conexão TCP**. O kernel demultiplexa pacotes que chegam em `:8080` baseado em quem é o `(IP_src, porta_src)` deles.

Cliente que faz `connect()` sem chamar `bind()` antes recebe uma porta efêmera (intervalo `net.ipv4.ip_local_port_range`):

```bash
sysctl net.ipv4.ip_local_port_range
# net.ipv4.ip_local_port_range = 32768 60999
```

Como portas efêmeras são distintas pra cada cliente, a tupla é distinta, e o kernel mantém as duas conexões em paralelo sem conflito.

## Conexão com teoria

Direto do `002_socket-tcp.md`:

> "Duas conexões TCP distintas ao mesmo servidor podem existir simultaneamente porque as portas de origem diferem."

Agora você viu impresso.

## Experimento extra

**Pergunta:** o que acontece se forçar o cliente a usar uma porta específica?

```bash
# em um cliente:
nc -p 50000 localhost 8080

# em outro cliente, tenta a mesma porta:
nc -p 50000 localhost 8080
# erro: Address already in use
```

A tupla seria a mesma — colisão. Por isso porta de origem é única **por par destino**.

## Dica de Docker

Os 3 shells (`server` + 2 `nc`) estão todos no **mesmo container**. Compartilham PID, MNT, NET — são processos peers. `docker exec` é exatamente isso: novo processo no namespace do container.

## Próximo

`03-accept/` — você viu 2 conexões aparecerem. Quantos sockets o servidor tem? Vamos contar.
