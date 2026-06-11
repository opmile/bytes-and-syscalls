# nc — netcat

O "canivete suíço" das redes. Abre, lê e escreve em sockets TCP/UDP direto do
shell. No `go-socket` ele faz papel de cliente quando você quer testar um
servidor sem escrever cliente em Go ainda.

> Existem implementações diferentes (`nc.openbsd`, `ncat` do nmap, `nc.traditional`).
> Flags variam. No container Debian/Alpine do lab você tipicamente tem a versão
> OpenBSD ou o `ncat`. `nc -h` mostra qual.

## Mental model

`nc` faz duas coisas: ou **conecta** num socket existente (cliente), ou **escuta**
em uma porta (servidor mínimo). Os dois lados ficam ligados pela tubulação
stdin↔socket↔stdout. Texto que você digita vira bytes na rede; bytes que chegam
viram texto na tela.

## Como cliente

```bash
nc 127.0.0.1 8080              # conecta TCP no servidor local
nc -v 127.0.0.1 8080           # verbose — mostra conexão estabelecida
nc -u 127.0.0.1 9000           # UDP
nc -z 127.0.0.1 8080           # "zero I/O" — só testa se porta abre. Sai 0/1.
nc -w 3 host 8080              # timeout de 3s na conexão
```

`-z` é o equivalente a `telnet host porta` mas scriptável — útil em healthcheck
de CI ou shell.

## Como servidor mínimo

```bash
nc -l 8080                      # escuta TCP em :8080 (OpenBSD nc)
nc -lk 8080                     # -k mantém aberto após cliente desconectar
nc -lvp 8080                    # `ncat`: verbose, listen, porta
```

Útil pra:
- Confirmar que um cliente Go está enviando o que você acha que está enviando.
- Simular servidor lento (`nc -l 8080 | pv -L 100`) — limita banda.
- Receber arquivo: `nc -l 8080 > recebido.bin` no servidor, `nc host 8080 < arquivo.bin` no cliente.

## Pipes — o uso mais subestimado

`nc` é programa de pipeline. Combina com qualquer coisa que produza/consuma stdout/stdin.

```bash
echo "ping" | nc 127.0.0.1 8080                  # manda 1 linha
printf "GET / HTTP/1.0\r\n\r\n" | nc host 80     # HTTP request à mão
cat payload.bin | nc -q1 host 9000               # envia binário, fecha após 1s ocioso
nc host 8080 < req.txt > resp.txt                # request salvo, resposta em arquivo
```

`-q1` (OpenBSD) força fechar 1s depois do EOF do stdin — sem isso, `nc` pode ficar
esperando dados eternamente do outro lado.

## Exemplos no contexto do go-socket

```bash
# estação 2 — duas conexões distintas pro mesmo :8080
nc 127.0.0.1 8080 &
nc 127.0.0.1 8080 &
ss -tn dst :8080            # vê duas linhas ESTABLISHED, portas de origem diferentes

# estação 3 — servidor Go com 1 listener, N accepts
for i in 1 2 3; do nc 127.0.0.1 8080 < /dev/null & done

# estação 5 — provoca TIME_WAIT
nc 127.0.0.1 8080 </dev/null     # cliente fecha primeiro → cliente em TIME_WAIT
ss -tn state time-wait
```

## Cuidado em produção

- `nc -l` sem autenticação na porta — qualquer um conecta. Use em local/lab.
- Versões diferentes: `nc -e /bin/sh` (executar comando ao conectar) só existe na
  versão `traditional` e é desabilitado por motivo óbvio. Ferramenta de pentest;
  em produção é sinal de invasão.
- `ncat` (do nmap) é a versão moderna mais consistente — vale instalar se você
  pula entre máquinas: `apt install ncat` / `brew install nmap`.

## socat — quando `nc` não chega

`socat` é o `nc` em esteroides. Faz tudo: TCP, UDP, Unix socket, TLS, serial,
exec, fork. Sintaxe `socat ADDR1 ADDR2` — conecta dois endpoints arbitrários.

```bash
socat - TCP:host:8080                         # tipo nc
socat TCP-LISTEN:8080,reuseaddr,fork EXEC:cat # echo server multi-cliente
socat UNIX-LISTEN:/tmp/s,fork TCP:host:80     # bridge unix→tcp
```

Curva mais íngreme, mas quando você precisa de TLS ad-hoc ou Unix socket no lab
do `go-net/unix/`, é ele.
