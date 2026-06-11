# tcpdump — captura de pacotes

`ss` mostra o **estado** das conexões. `tcpdump` mostra os **pacotes** indo e
voltando. Quando o comportamento da rede contradiz o que seu código acha que
está fazendo, é o tcpdump que resolve a discussão.

> Precisa de privilégio (`sudo`) porque acessa a interface em modo promíscuo.
> Em container, precisa de `--cap-add=NET_ADMIN` ou `--privileged`.

## Mental model

`tcpdump` se conecta a uma interface de rede (`lo`, `eth0`, etc.) e imprime cada
pacote que passa. Você decide:
1. Em **qual interface** ouvir (`-i`).
2. **Quais pacotes** mostrar (filtro BPF — última parte da linha de comando).
3. **Quanto detalhe** imprimir (`-v`, `-vv`, `-X`).

```
14:23:01.123456 IP 172.18.0.3.51234 > 172.18.0.2.8080: Flags [S], seq 12345, win 65535, length 0
14:23:01.123789 IP 172.18.0.2.8080 > 172.18.0.3.51234: Flags [S.], seq 67890, ack 12346, win 65535, length 0
14:23:01.123900 IP 172.18.0.3.51234 > 172.18.0.2.8080: Flags [.], ack 67891, win 65535, length 0
```

Isso é o handshake TCP de 3 vias (SYN, SYN-ACK, ACK) — a coisa que todo livro
desenha. Você acabou de **ver** acontecer.

## Flags essenciais

```bash
-i any            # todas as interfaces (Linux)
-i lo             # loopback (testes locais — go-socket vive aqui)
-i eth0           # interface específica
-n                # não resolve IP
-nn               # não resolve IP nem porta (recomendado em geral)
-v / -vv / -vvv   # mais detalhe
-X                # mostra payload em hex + ASCII
-A                # payload só ASCII (útil para HTTP cru)
-s 0              # captura pacote inteiro (default trunca em ~262KB no moderno; em versões antigas era 68 bytes)
-c 10             # captura 10 pacotes e sai
-w captura.pcap   # salva binário (abre depois em Wireshark)
-r captura.pcap   # lê arquivo salvo
-S                # números de sequência absolutos (não relativos)
-tttt             # timestamp legível (com data)
```

## Filtro BPF — onde a ferramenta brilha

A última parte do comando é um filtro. A sintaxe é própria, simples:

```bash
tcpdump -nn -i lo port 8080
tcpdump -nn -i lo tcp port 8080
tcpdump -nn -i any host 172.18.0.2
tcpdump -nn -i any src 10.0.0.5 and dst port 443
tcpdump -nn -i any 'tcp[tcpflags] & tcp-syn != 0'   # só pacotes com SYN
tcpdump -nn -i any 'tcp[tcpflags] & (tcp-fin|tcp-rst) != 0'  # FIN ou RST
tcpdump -nn -i any net 10.0.0.0/8
tcpdump -nn -i any not port 22                       # ignora barulho do ssh
```

Operadores: `and`, `or`, `not`. Termos: `host`, `net`, `port`, `src`, `dst`,
`tcp`, `udp`, `icmp`, `portrange 1024-2048`.

## Exemplos no contexto do go-socket

```bash
# estação 0 — o que passa em loopback quando seu server aceita conexão?
sudo tcpdump -nn -i lo port 8080

# estação 4 — ver o flow control freando o sender
sudo tcpdump -nn -i lo port 8080 -v       # observar `win 0` quando receive buffer enche

# estação 5 — capturar o fechamento e ver quem manda FIN primeiro
sudo tcpdump -nn -i lo port 8080 'tcp[tcpflags] & (tcp-fin|tcp-rst) != 0'

# salvar pra abrir no Wireshark depois (Wireshark roda no macOS, lê o .pcap)
sudo tcpdump -nn -i any -w /tmp/lab.pcap port 8080
```

## Lendo a saída

Cada linha tem um padrão:

```
[timestamp]  IP  [src].[port]  >  [dst].[port]:  Flags [....], seq N, ack N, win N, length N
```

Flags resumidas — você vai memorizar:

| Símbolo | Flag    | Quando aparece                              |
|---------|---------|---------------------------------------------|
| `S`     | SYN     | abertura de conexão                         |
| `S.`    | SYN+ACK | servidor respondendo SYN                    |
| `.`     | ACK puro| confirmação sem dados                       |
| `P.`    | PSH+ACK | dados + flush ("entrega pra aplicação já")  |
| `F.`    | FIN+ACK | fechamento ordenado                         |
| `R`     | RST     | fechamento abrupto (erro/abort)             |

`length` = bytes de payload (não inclui headers). Pacote com `length 0` é só
sinalização (handshake, ACK).

## tcpdump em container

No lab do `go-socket`, dentro do container você precisa:
```bash
apt install -y tcpdump        # se a imagem não tem
tcpdump -nn -i any port 8080  # `any` captura inclusive `lo` dentro do netns
```

E o container precisa ter `NET_ADMIN`:
```yaml
services:
  inspector:
    cap_add: ["NET_ADMIN"]
```

## Workflow recomendado

1. **Capture no servidor, leia no laptop.** `tcpdump -w` no Linux → `scp` o
   `.pcap` → abre no Wireshark no macOS. Filtros e UI do Wireshark valem ouro
   pra investigação demorada.
2. **Em produção, sempre `-c` ou tempo limitado.** Captura sem limite em
   servidor cheio enche disco rápido.
3. **Cuidado com SSH.** Capturar tudo na interface principal inclui o seu
   próprio tráfego SSH e polui a saída. Sempre `not port 22`.
4. **Combine com `-X` quando o payload é texto.** HTTP/1, Redis, SMTP — você
   lê o protocolo direto.

## Wireshark / tshark

`tshark` é o `tcpdump` da família Wireshark — mesma engine, filtros mais
poderosos. Wireshark (GUI) é a ferramenta de análise séria. Para captura ad-hoc
em servidor, `tcpdump` está em toda parte; para análise pós-mortem com
follow-stream, gráficos, decodificação de protocolos, abre o `.pcap` no
Wireshark.
