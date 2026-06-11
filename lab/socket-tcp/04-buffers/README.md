# Estação 04 — `write()` escreve no buffer, não na rede

## Pergunta-âncora

Cliente faz `conn.Write(payload_grande)`. Receiver não está lendo. O que acontece com `Write`? Bloqueia? Em quantos bytes? Por quê?

## Hipótese antes de rodar

- Que parte é instantânea, que parte bloqueia?
- Se o receiver não lê, onde os bytes ficam?
- Quanto cabe antes de bloquear?

## Rodar

**Shell A — server (slow-reader):**
```bash
docker exec -it socket-tcp-server bash
cd /app/04-buffers
go run server.go
```

Aguarda mensagem "Aguarde cliente conectar...".

**Shell B — inspector (observador):**
```bash
docker exec -it socket-tcp-inspector bash
watch -n 0.5 'ss -tn | grep 8080'
```

Mantém isso rodando enquanto o resto acontece.

**Shell C — cliente (write-burst):**
```bash
docker exec -it socket-tcp-server bash
cd /app/04-buffers
go run client.go
```

Veja o contador de bytes escritos.

## O que observar

### No cliente

```
total escrito: 4096 bytes (4 KB)
total escrito: 1048576 bytes (1024 KB)
total escrito: 3145728 bytes (3072 KB)
total escrito: 3145728 bytes (3072 KB)   <- congelou
total escrito: 3145728 bytes (3072 KB)   <- ainda congelado
...
```

O número cresce rápido até um ponto, **congela**, e fica parado. Esse é o momento em que o send buffer encheu e `conn.Write()` está bloqueado.

### No inspector (`ss -tn`)

```
State  Recv-Q  Send-Q  Local Address:Port  Peer Address:Port
ESTAB  XXXXXX  YYYYYY  127.0.0.1:8080      127.0.0.1:51234   <- lado server
ESTAB  0       ZZZZZZ  127.0.0.1:51234     127.0.0.1:8080   <- lado client
```

- **Recv-Q** do servidor cresce até o limite do receive buffer e estaciona. Esses são bytes que o TCP recebeu mas o app (que está dormindo) não leu ainda.
- **Send-Q** do cliente cresce até o limite do send buffer e estaciona.
- A soma `Recv-Q server + Send-Q client` é o "in-flight + buffered" total que congelou o cliente.

### Após 15s, server acorda — e drena em pulsos

O server não engole tudo de uma vez. Ele lê 4 MB, pausa 1s, repete:

```
Acordei. Drenando em PULSOS (lê 4MB, pausa 1s)...
drenado: 4194304 bytes (4 MB)
drenado: 8388608 bytes (8 MB)
drenado: 12582912 bytes (12 MB)
...
Drenei 67108864 bytes em ~16s. err=EOF
```

**Esse é o gráfico vivo.** Olhando o `ss` do inspector durante o drain:

- **Recv-Q do server despenca pra ~0** a cada pulso (o `ReadFull` esvazia o recv
  buffer) e **reenche durante a pausa de 1s** (o client, ainda bloqueado com mais
  dados, joga 4 MB assim que a janela abre). Sobe e desce, sobe e desce.
- **Send-Q do client acompanha**: cai quando o server lê, enche de novo na pausa.
  Os dois lados oscilam **em conjunto** — é o flow control abrindo e fechando a
  janela em tempo real.

No cliente, o contador volta a crescer, mas **em degraus** de ~4 MB a cada ~1s,
não de uma vez — agora o reader lento é o gargalo, não mais o buffer cheio.

No fim, `err=EOF` (ou `unexpected EOF` se o último pulso for parcial): o client
escreveu os 64 MB, imprimiu `Escrevi ... Fechando conn` e fechou. É esse
fechamento que termina o loop de `ReadFull` do server — sem a quota finita, o
server ficaria preso lendo pra sempre.

### Conferindo os limites

```bash
sysctl net.ipv4.tcp_wmem
# net.ipv4.tcp_wmem = 4096	16384	4194304
#                     min   default  max

sysctl net.ipv4.tcp_rmem
# net.ipv4.tcp_rmem = 4096	131072	6291456
```

Os 3 valores: `min`, `default`, `max`. O kernel ajusta dinamicamente entre default e max conforme uso. O total que congelou no seu teste deve estar próximo de `default + max` (send do client + receive do server).

## Resposta esperada

`Write` **não escreve na rede**. Escreve no **send buffer do kernel** e retorna. TCP, em background, segmenta e envia.

Se receiver não lê:
1. Receive buffer do kernel do servidor enche.
2. TCP flow control: ACK do servidor anuncia `window=0`.
3. TCP do cliente para de enviar. Send buffer do cliente enche.
4. Próximo `Write` bloqueia até abrir espaço.

A goroutine que chamou `Write` fica parada — **I/O bound**. CPU ociosa, thread (do ponto de vista do scheduler Go) cedida pra outras goroutines.

## Conexão com teoria

Direto do `002_socket-tcp.md`:

> "Você não escreve direto na rede. Escreve no buffer do kernel, e o TCP cuida do resto de forma assíncrona."
>
> "O `write()` em si raramente bloqueia — só bloqueia se o send buffer estiver cheio."

Esta estação **prova quantitativamente**: você viu o N de bytes no qual bloqueou.

O que esta estação prova é backpressure por **flow control** (o receiver te
freia). Existe um irmão — backpressure por **congestionamento de rede**
(congestion control) — que loopback não consegue mostrar. Ver `notes.md`
(seção "Flow control × congestion control") para a distinção completa.

## Por que isso importa pra Go

`io.Copy(dst, src)` e `conn.Write` parecem síncronos, mas na prática só bloqueiam em condições específicas (buffer cheio, receiver lento, rede congestionada). O scheduler Go troca goroutines automaticamente quando isso acontece. Esse é o "epoll por baixo, parece síncrono em cima" do `goroutines-x-tomcat.md`.

## Experimento extra

Reduza o send buffer:

```bash
# Tem que estar como root e privilegiado pra alterar (Docker normal não permite).
# Alternativa: setsockopt SO_SNDBUF no cliente. Adicionar antes do Dial:
#   conn.(*net.TCPConn).SetWriteBuffer(64 * 1024)
```

Com buffer menor, o congelamento acontece mais cedo. Prova que o limite vem do kernel/socket option, não do código de aplicação.

## Próximo

`05-states/` — agora que conexões existem e bytes fluem, vamos ver os estados TCP no fechamento.
