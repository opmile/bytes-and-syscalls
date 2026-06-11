# ss — socket statistics

`ss` é o substituto moderno do `netstat`. Lê direto do kernel via `netlink`, então é
ordens de magnitude mais rápido em máquinas com muitas conexões. Em qualquer Linux
recente (incluindo o container do `go-socket`) é a ferramenta padrão para inspecionar
sockets.

> Linux-only. macOS não tem — por isso o lab do `go-socket` roda em container.

## Mental model

Cada linha do `ss` é **um socket**. Para TCP, ela mostra:

```
State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port
ESTAB   0       0       172.18.0.2:8080     172.18.0.3:51234
```

- `State` — estado da state machine TCP (`LISTEN`, `ESTAB`, `TIME_WAIT`, `CLOSE_WAIT`...).
- `Recv-Q` / `Send-Q` — bytes pendentes no receive/send buffer do kernel. Se `Send-Q`
  vive alto, sender está sendo freado por flow control (estação 4 do guia).
- `Local:Port` + `Peer:Port` — duas metades da 4-tupla. Se duplicar `Local`, são
  conexões distintas só na porta de origem do outro lado.

## Flags que você vai usar sempre

```bash
-t    # TCP
-u    # UDP
-x    # Unix sockets (IPC local — relevante pro `go-net/unix/`)
-l    # só listeners
-a    # tudo, inclusive listeners (default esconde)
-n    # não resolver porta nem IP (mais rápido, mais legível)
-p    # mostra processo dono (precisa permissão; combina com sudo)
-o    # mostra timers (retransmissão, keepalive, time_wait restante)
-e    # estendido — inclui inode, uid, sk cookie
-i    # info do TCP interno (cwnd, rtt, retrans...)
```

Combinação padrão de bolso: `ss -tnp` (TCP, numérico, com PID).

## Exemplos no contexto do go-socket

```bash
ss -tlnp                          # quem está escutando TCP nesta máquina?
ss -tn                            # conexões TCP ativas, numéricas
ss -tan                           # inclui LISTEN + TIME_WAIT + tudo
ss -tnp dst :8080                 # quem conecta no servidor da estação 2
ss -tn state time-wait            # vê o TIME_WAIT da estação 5
ss -tn state close-wait           # caça `CLOSE_WAIT` (bug de close esquecido)
ss -tne                           # com inode — casa com /proc/net/tcp da estação 6
ss -s                             # sumário geral (quantos sockets por estado)
```

## Filtros — sintaxe poderosa pouco conhecida

```bash
ss -tn '( dport = :80 or dport = :443 )'
ss -tn 'src 10.0.0.0/8'
ss -tn 'state established and dport > :1024'
```

A linguagem de filtro aceita `src/dst`, portas, redes, estados. Útil em produção
quando `ss -tan | grep` vira ruído.

## Atalhos mentais

- `ss -tlnp` → "o que está aberto nesta máquina e qual processo?"
- `ss -tnp` → "com quem este host fala agora?"
- `ss -s` → "quantos sockets, distribuídos como?"
- `ss -tni` → "esta conexão lenta — está perdendo pacote? `retrans` aparece aí."

## Relação com `/proc/net/tcp`

`ss` é uma view formatada do mesmo dado que `cat /proc/net/tcp` te mostra cru. A
estação 6 do `go-socket` te faz reimplementar parte do que `ss -p` faz: parsear
a tabela e cruzar `inode` com `/proc/<pid>/fd/`. Depois disso, `ss` deixa de ser
mágica.

## netstat (legado)

Você vai ver `netstat -tnlp` em tutoriais antigos. Faz a mesma coisa, mas
varre `/proc` em vez de usar netlink — lento em servidor cheio. Não desinstala,
mas o reflexo novo é `ss`.
