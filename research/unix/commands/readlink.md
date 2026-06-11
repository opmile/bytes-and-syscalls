# readlink — ler o alvo de um symlink

`readlink` recebe um symlink e imprime **para onde ele aponta** — não segue, não
abre. Uma linha de saída, sem mais nada. É a ferramenta certa quando você
precisa do destino do link como **dado** (pra script, comparação, parsing), não
do conteúdo do arquivo apontado.

## Mental model

```
arquivo.txt        →  arquivo regular, contém bytes
link.txt           →  symlink, contém "arquivo.txt" como string
```

- `cat link.txt` → segue o link, mostra bytes de `arquivo.txt`.
- `readlink link.txt` → imprime literalmente `arquivo.txt`.
- `ls -l link.txt` → mostra `link.txt -> arquivo.txt` (formato humano).

`readlink` é o jeito **scriptável** de pegar o alvo.

## Flags úteis

```bash
readlink link                    # alvo cru, exatamente como gravado
readlink -f link                 # caminho absoluto canônico, segue cadeia de symlinks
readlink -e link                 # como -f, mas falha se algum componente não existir
readlink -m link                 # como -f, mas não falha se não existir (útil pra "qual seria o path")
```

> macOS tem `readlink` mas sem `-f` no BSD original. Use `greadlink` (`brew install coreutils`) ou `realpath`. No container Linux do lab, `-f` funciona direto.

## Por que importa pro lab `go-socket`

Tudo no `/proc` que aponta pra recurso do kernel é **symlink mágico**. O texto
do link não é caminho real — é um identificador. `readlink` te entrega esse
identificador limpo.

### Caso 1 — sanity check NET namespace compartilhado

Cada namespace do kernel aparece como symlink em `/proc/self/ns/`. O alvo é uma
string `tipo:[inode]`, onde o **inode** identifica univocamente o namespace.

```bash
docker exec socket-tcp-server    readlink /proc/self/ns/net
# net:[4026532567]

docker exec socket-tcp-inspector readlink /proc/self/ns/net
# net:[4026532567]
```

**Inodes iguais = mesmo NET namespace.** O `network_mode: "service:server"` do
`docker-compose.yml` funcionou. Se os inodes diferirem, inspector está em
namespace próprio — `ss`/`tcpdump` dele não veem os sockets do server.

Compara com PID namespace pra ver o contraste:

```bash
docker exec socket-tcp-server    readlink /proc/self/ns/pid
# pid:[4026532570]
docker exec socket-tcp-inspector readlink /proc/self/ns/pid
# pid:[4026532580]   ← diferente
```

NET compartilhado, PID isolado. Por isso inspector vê os sockets via
`/proc/net/tcp` mas **não** vê os PIDs do server em `ps`. Estação 6 do guia
depende dessa assimetria — o lookup inode→PID precisa rodar **dentro do
server**, não do inspector.

Script de sanity em uma linha:

```bash
[ "$(docker exec socket-tcp-server readlink /proc/self/ns/net)" = \
  "$(docker exec socket-tcp-inspector readlink /proc/self/ns/net)" ] \
  && echo "NET ns compartilhado ✓" || echo "NET ns DIFERENTE ✗"
```

### Caso 2 — socket → inode (estação 1)

`/proc/<pid>/fd/<N>` é symlink mágico para `socket:[<inode>]`:

```bash
readlink /proc/$PID/fd/3
# socket:[165592]
```

Esse `165592` é o mesmo inode que aparece em `/proc/net/tcp` na coluna `inode`.
É a chave global pra identificar **este** socket no kernel. `ls -l` mostra
visualmente; `readlink` te dá a string crua pra extrair o inode com `sed`/`awk`:

```bash
readlink /proc/$PID/fd/3 | sed 's/socket:\[\(.*\)\]/\1/'
# 165592
```

### Caso 3 — mapeamento reverso (estação 6)

Achou inode `165592` em `/proc/net/tcp` e quer saber qual processo dono:

```bash
sudo find /proc -lname "socket:\[165592\]" 2>/dev/null
# /proc/905/fd/3
```

`find -lname` casa contra o alvo do symlink. Cada match é
`/proc/<pid>/fd/<N>` — extrai PID e FD do caminho. Implementação em Go da
estação 6 faz exatamente este loop: lê `os.Readlink` em cada `/proc/*/fd/*`,
compara com a string `socket:[N]`.

## Outros symlinks mágicos do `/proc` que valem `readlink`

```bash
readlink /proc/self/exe        # binário rodando ("/usr/local/go/bin/go" ou seu server)
readlink /proc/self/cwd        # diretório de trabalho do processo
readlink /proc/self/root       # raiz vista pelo processo (chroot detector)
readlink /proc/self/fd/0       # de onde vem o stdin ("/dev/pts/0" ou pipe)
readlink /proc/self/fd/1       # pra onde vai o stdout
```

Diagnóstico rápido "este processo está logando em arquivo deletado?":

```bash
readlink /proc/$PID/fd/1
# /var/log/app.log (deleted)
```

`(deleted)` aparece quando o arquivo foi rotacionado/removido mas o processo
ainda segura o FD — bug clássico de log que para de aparecer no `tail`.

## `readlink -f` vs `realpath`

```bash
readlink -f ~/code/workspaces/go-family/go-socket
realpath  ~/code/workspaces/go-family/go-socket
# /Users/milenaoliveirapenhalves/code/go/go-socket
```

Os dois resolvem cadeia inteira de symlinks até o caminho real. `realpath` é
mais portável (existe em macOS por padrão em versões recentes); `readlink -f` é
GNU. Pra resolver os symlinks da família `go-family` (que apontam pra
`~/code/go/`), qualquer um serve.

## `readlink` vs `ls -l` vs `stat`

| Ferramenta     | O que dá                                        | Quando usar               |
|----------------|-------------------------------------------------|---------------------------|
| `ls -l link`   | `link -> alvo` em texto humano                  | inspeção visual rápida    |
| `readlink link`| só `alvo`, uma linha                            | script, parsing           |
| `stat link`    | metadata completa (tipo, inode, perms, mtime…)  | quando precisa mais que destino |
| `stat -L link` | igual, mas resolvendo o symlink                 | metadata do **alvo**      |

## Pegadinhas

- `readlink arquivo-comum` (não-symlink) sai com código `1` e nada no stdout.
  Em script, sempre cheque `$?` ou use `readlink -f` que devolve o próprio
  caminho pra arquivos regulares.
- `readlink` lê **um** nível por padrão. Se `a -> b -> c`, `readlink a` dá `b`.
  Para chegar em `c` use `-f`.
- `find -lname PADRÃO` filtra symlinks pelo alvo — combinação natural com
  `readlink` quando você quer caçar todos os links que apontam pra algo.
