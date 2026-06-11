# /proc, namespaces, inodes, readlink

Conceitos de Linux que apareceram nas verificações de sanidade do Passo 5 e voltam nas estações (especialmente `06-mapping`, que usa `os.Readlink` sobre `/proc/<pid>/fd/`).

Nota de referência — não é tutorial sequencial, é dicionário curto.

## `/proc/self/ns/net`

`/proc` = filesystem virtual do Linux. Não mora em disco. Kernel gera arquivos sob demanda quando alguém lê. Cada um é uma "janela" pra estado interno do kernel.

Decomposição do path:

| Pedaço      | Significado                                                                 |
|-------------|-----------------------------------------------------------------------------|
| `/proc/`    | Filesystem virtual do kernel                                                |
| `/<pid>/`   | Diretório por processo (ex: `/proc/1234/`)                                  |
| `/self/`    | Atalho — sempre aponta pro processo que está lendo. Evita ter que saber o PID. `/proc/self/` ≡ `/proc/<meu-pid>/` |
| `/ns/`      | Diretório com os namespaces do processo                                     |
| `/net`      | Namespace de rede. Há também `pid`, `mnt`, `uts`, `user`, `ipc`, `cgroup`, `time` |

`/proc/self/ns/net` = "qual NET namespace o processo atual habita".

Cada arquivo em `/ns/` é um **symlink especial** apontando pra algo tipo `net:[4026532XXX]`. O número entre colchetes é o **inode** do namespace.

## Inodes

Conceito de filesystem Unix. Toda "coisa" no sistema (arquivo, diretório, socket, namespace, etc) tem um **inode** = identificador numérico único dentro do seu domínio.

No disco:
- Nome do arquivo (`README.md`) mora no diretório.
- Conteúdo + metadados (tamanho, permissões, dono, timestamps) moram no **inode**.
- Diretório guarda mapping `nome → inode`.

Dois arquivos diferentes = dois inodes diferentes. Mesmo arquivo com dois nomes (hard link) = um inode só.

Kernel reusa o conceito pra coisas virtuais:
- **Socket**: cada socket tem inode (`socket:[12345]`). É o que aparece em `/proc/<pid>/fd/<N>` na estação 06.
- **Namespace**: cada namespace tem inode (`net:[4026532XXX]`). Dois processos com o mesmo inode em `/proc/self/ns/net` estão **dentro do mesmo NET namespace**.

Inode = "identidade" do recurso. Compará-los responde "é a mesma instância?".

`ls -i <arquivo>` mostra inode de qualquer arquivo:

```bash
ls -i /app/Dockerfile
# 1234567 /app/Dockerfile
```

## `readlink`

Comando Unix que lê o **alvo** de um symlink. Não segue o link, não abre nada — só imprime pra onde aponta.

```bash
# se /tmp/atalho aponta pra /etc/hosts:
readlink /tmp/atalho
# /etc/hosts
```

Por que importa pra namespace? Os arquivos em `/proc/self/ns/*` são **symlinks especiais** cujo alvo é uma string tipo `net:[4026532898]`. Não são symlinks pra outro arquivo do filesystem — kernel hackeou eles pra carregar o inode como texto.

```bash
readlink /proc/self/ns/net
# net:[4026532898]
```

Comparando essa string em dois containers, sei se compartilham o namespace. Mesmo texto = mesmo inode = mesmo namespace = mesma instância no kernel.

Alternativa equivalente: `ls -la /proc/self/ns/net` mostra o alvo do symlink na coluna final. `readlink` é só mais limpo.

## Onde isso reaparece nas estações

| Conceito       | Estação                                                                                          |
|----------------|--------------------------------------------------------------------------------------------------|
| `/proc/net/tcp`| 04, 05, 06 — tabela bruta de sockets TCP IPv4 vista pelo NET namespace atual                     |
| `/proc/<pid>/fd/` | 01, 06 — symlinks tipo `socket:[<inode>]`, lidos com `os.Readlink` no código Go               |
| Inode de socket | 06 — chave pra mapear socket → PID (algoritmo do `ss -p`)                                       |
| `/proc/self/ns/` | Passo 5 — sanidade de NET ns compartilhado e PID ns isolado entre containers                  |

## Resumo de uma frase

`readlink /proc/self/ns/net` = "imprime o inode do NET namespace que estou habitando, sem efeitos colaterais".
