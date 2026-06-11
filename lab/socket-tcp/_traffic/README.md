# _traffic — gerador de tráfego pra observar a estação 06

Helper, não é estação (daí o prefixo `_`). Existe pra dar ao `06-mapping` uma
tabela cheia de sockets com PID conhecido, sem depender de subir outra estação.

## Por que existe

Rodar o `06-mapping` num sistema parado mostra pouco: poucos sockets, e os que
sobram de processos mortos (ex: `TIME_WAIT` de um `nc` desconectado) aparecem
com `PID = -` porque não têm processo dono vivo. Isso esconde o que a estação
quer ensinar — o JOIN inode → PID funcionando.

Este programa é um único processo que segura, vivo, `1 + 2N` sockets, todos do
**mesmo PID** — o mapper resolve todos.

## Por que `1 + 2N`

Conta por origem de cada socket:

| Qtd | De onde vem | Estado |
|---|---|---|
| **1** | `net.Listen` — o socket que aceita conexões | `LISTEN` |
| **N** | `net.Dial` rodado N vezes — a ponta **cliente** | `ESTABLISHED` |
| **N** | `ln.Accept` retorna uma vez por Dial — a ponta **servidor** | `ESTABLISHED` |

`1 + N + N = 1 + 2N`.

O `2N` é o ponto conceitual: **uma conexão TCP tem duas pontas, e cada ponta é
um socket separado** — FD e inode próprios. A 4-tupla (estação 02) descreve a
conexão; cada lado dela é um objeto de kernel distinto. Normalmente cliente e
servidor vivem em processos/máquinas diferentes. Aqui o processo disca em si
mesmo, então as duas pontas caem no mesmo PID — por isso cada conexão aparece
**duas vezes** na tabela do mapper (`:9090` de um lado, porta efêmera do outro).

O `1` do listener é à parte: não é ponta de conexão nenhuma, é o socket passivo
que só pare conexões novas. Soma um, não dois.

## Rodar

Dois shells, **ambos no server** (PID namespace tem que ser o mesmo do mapper):

```bash
# Shell 1 — sobe o tráfego e deixa vivo
docker exec -it socket-tcp-server bash
cd /app/_traffic && go run main.go
# imprime o PID e segura. NÃO feche.
```

```bash
# Shell 2 — roda o mapper enquanto o tráfego está vivo
docker exec -it socket-tcp-server bash
cd /app/06-mapping && go run main.go
```

Esperado: várias linhas em `127.0.0.1:9090`, todas com o mesmo PID e `COMM`
igual ao binário do tráfego. Compare com `ss -tnp | grep 9090`.

Ctrl-C no shell 1 fecha tudo (os `defer`/Close derrubam os sockets).

## Ajustar volume (`conns`, default 3)

`conns` (= N) no topo do `main.go` controla quantos pares abrir. O critério do
default tem dois limites:

- **Piso:** > 1, pra a tabela mostrar **repetição**. Ver várias linhas
  resolvendo pro mesmo PID é o que torna o JOIN visível; com N=1 poderia parecer
  coincidência.
- **Teto:** pequeno o bastante pra a saída caber na tela e você ler linha a
  linha. O objetivo é didático, não estresse — não adianta floodar.

N=3 dá 7 sockets (`1 + 6`): suficiente pro padrão das pontas pareadas, enxuto
pra ler. Suba pra ver a tabela crescer.
