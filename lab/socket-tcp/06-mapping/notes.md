# Socket → processo: o JOIN via inode

Como `ss -tnp` descobre qual processo é dono de cada socket. Não há mágica: é
um JOIN entre duas tabelas de `/proc`, feito à mão. Esta nota costura a leitura
do `main.go`, do parsing de endereço aos idiomas de Go que confundem na
primeira leitura.

## O problema: duas tabelas que não se conversam

O kernel mantém dois subsistemas separados, cada um com sua visão do sistema.
Nenhum dos dois sabe o que o outro sabe:

| Fonte | O que tem | O que falta |
|---|---|---|
| `/proc/net/tcp` | todos os sockets: endereço, estado, inode | **quem é o dono** (nenhum PID) |
| `/proc/<pid>/fd/` | os FDs de cada processo, com seu inode | **o que é cada socket** (endereço, estado) |

`/proc/net/tcp` é a tabela do subsistema de **rede**. `/proc/<pid>/fd/` é a do
subsistema de **processos**. São mundos que o kernel não cruza por você.

## A chave-estrangeira: o inode

O `inode` do socket aparece nas **duas** fontes — é a coluna de junção. Pensando
em banco:

```sql
SELECT t.local, t.remote, t.state, t.inode,  -- da tabela de rede
       f.pid, f.comm                          -- da tabela de processos
FROM   proc_net_tcp t
LEFT JOIN proc_pid_fd f ON t.inode = f.inode
```

O `LEFT JOIN` importa: um socket pode existir em `/proc/net/tcp` sem dono
visível (kernel-owned, processo morto, ou outro namespace). Esses ficam com
`pid = -1`, impressos como `-`.

Por que o inode serve de identidade? Cada socket ganha um inode único na
criação. O kernel grava esse mesmo número na tabela de rede **e** no symlink do
FD (`socket:[<inode>]`). Mesmo número nos dois lugares ⟹ mesmo objeto kernel,
visto de dois ângulos. Ver `notebook/003_proc-namespaces-inodes-readlink.md`.

## O algoritmo em três movimentos

```
parseProcNetTCP()  →  lista de sockets, pid = -1   (lado esquerdo do JOIN)
buildInodeIndex()  →  mapa inode → (pid, comm)      (lado direito)
main()             →  casa os dois pelo inode, imprime
```

### 1. `parseProcNetTCP` — lê o lado "rede"

Lê `/proc/net/tcp` inteiro, pula o header (`lines[1:]`), quebra cada linha em
campos com `strings.Fields` (colapsa espaços múltiplos). As colunas são
posicionais, sem nome:

| Índice | Campo |
|---|---|
| `fields[1]` | endereço local |
| `fields[2]` | endereço remoto |
| `fields[3]` | estado (hex) |
| `fields[9]` | **inode** (coluna 10) |

O guard `if len(fields) < 10 { continue }` pula a linha vazia final do arquivo,
que senão daria panic em `fields[9]`. Cada entry nasce com `pid: -1` (dono a
preencher depois).

O estado vem em hex (`0A`, `01`), traduzido por um mapa fixo `tcpStates`
(`0A → LISTEN`, `01 → ESTABLISHED`, ...). O kernel guarda o estado TCP como enum
numérico e expõe em hexadecimal.

### 2. `parseAddr` — a parte densa: endianness

Entrada `"0100007F:1F90"` → saída `"127.0.0.1:8080"`. IP e porta seguem
convenções **diferentes**:

**IP — bytes em little-endian, reverte byte a byte.**

```
"0100007F"  →  hex.DecodeString  →  []byte{1, 0, 0, 127}
                                       índice 0 1 2 3
net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
        = 127 . 0 . 0 . 1   ✓
```

O kernel grava o IP na ordem que a máquina (x86, little-endian) o tem na
memória: do byte menos significativo pro mais significativo. Little-endian é
**por byte**, então basta reverter a ordem dos 4 bytes. Por isso os índices
`3,2,1,0`.

**Porta — número em big-endian, lê direto.**

```go
port, _ := strconv.ParseInt("1F90", 16, 32)  // = 8080
```

A porta está em network byte order (big-endian), que é como um humano lê um hex
da esquerda pra direita: `0x1F90 = 8080`. Sem inversão. (Args do `ParseInt`:
base 16, saída de 32 bits.)

> Resumo da assimetria: **IP inverte (little-endian, byte a byte); porta não
> inverte (big-endian, lê direto).** Convenções distintas no mesmo arquivo.

### 3. `buildInodeIndex` — lê o lado "processo"

Monta o mapa `inode → (pid, comm)`, o lado direito do JOIN.

- `filepath.Glob("/proc/[0-9]*")` filtra só os diretórios numéricos (um por
  PID). `/proc` também tem nomes textuais (`net`, `self`, `cpuinfo`); o glob e o
  `strconv.Atoi` seguinte peneiram só os PIDs.
- `os.ReadDir("/proc/<pid>/fd")` lista os FDs. Pode falhar por permissão se o
  processo for de outro usuário — o `continue` engole o erro. (No container como
  root, não acontece.)
- O nome do processo vem de `/proc/<pid>/comm`, lido **uma vez por processo**
  (igual pra todos os FDs daquele PID).
- Para cada FD, `os.Readlink` lê o **alvo do symlink** sem segui-lo. Para socket,
  devolve `socket:[12345]`. O regex `^socket:\[(\d+)\]$` casa a string inteira e
  captura o número no grupo 1 (`m[1]`). Alvos que não são socket (arquivo, pipe)
  devolvem `nil` → ignorados.

O FD como número inteiro que aponta pra `socket:[<inode>]` é o conceito da
estação `01-fd`.

### 4. `main` — o JOIN

```go
for i := range entries {
    if owner, ok := index[entries[i].inode]; ok {
        entries[i].pid = owner.pid
        entries[i].cmd = owner.cmd
    }
}
```

Três idiomas de Go empilhados aqui:

**`for i := range entries` — só o índice.** `range` num slice dá `(índice,
elemento)`. Com uma variável só, vem o **índice**. Usa-se o índice de propósito:
`entries[i].pid = ...` escreve **direto no slice**. Se fosse
`for _, e := range`, `e` seria uma **cópia** da struct e a mutação se perderia no
fim da iteração (Go copia structs por valor).

**`if owner, ok := ... ; ok` — if com statement + comma-ok.** O `;` separa um
statement (roda primeiro) da condição (testada depois). O statement é o
comma-ok de map: `valor, existe := mapa[chave]`. `ok` é `bool` — `true` se a
chave existe. Resolve a ambiguidade do `null` do Java (chave ausente vs. valor
nulo guardado). `owner` e `ok` têm escopo só dentro do `if`.

**As duas atribuições — o JOIN propriamente.** `entries[i]` tem rede preenchida
e dono vazio (`pid=-1`, `cmd=""`); `owner` tem só `{pid, cmd}`. As linhas
transplantam os dois campos que faltavam, preservando `local/remote/state/inode`.
Não dá pra fazer `entries[i] = owner`: são tipos diferentes (`socketEntry` de 6
campos vs. struct anônima de 2), e jogar por cima apagaria os outros quatro.

Se `ok == false` (inode sem dono no índice), as atribuições **não rodam**: `pid`
fica -1, impresso como `-`. É o LEFT JOIN — a linha da esquerda sobrevive sem
par na direita.

## A pegadinha de namespace (por que rodar no server, não no inspector)

Os containers compartilham **NET namespace** mas têm **PID namespaces**
separados (ver `notebook/001_shared-network-vs-shared-namespace.md`):

- `/proc/net/tcp` é estado de **rede** → server e inspector veem a **mesma**
  tabela.
- `/proc/<pid>/fd/` é estado de **processo** → cada um vê só os **seus** PIDs.

Rodar o mapper no inspector: o passo 1 acha os sockets, mas o passo 2 não
enxerga os PIDs do server → o JOIN não casa nada e tudo vira `-`. **Rode no
server.** A lição: o inode só liga as duas tabelas se ambas forem do **mesmo
namespace**. Identidade de kernel é relativa ao namespace.

## Resumo de uma frase

Lê a tabela de rede (sockets sem dono) → varre todos os processos montando
`inode → PID` → casa os dois pelo inode → imprime. O `ss -p` em ~160 linhas.
