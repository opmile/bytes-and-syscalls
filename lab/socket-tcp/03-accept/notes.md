# Sobre o Connected Socket

**O servidor cria um NOVO socket a cada conexão aceita.** Ele jamais reusa o socket original que está escutando.

---

### 1. A Dinâmica do `accept()`: Criando vs. Reusando

Imagine se o servidor **reusasse** o socket original (o *listener*). Se o socket que está configurado para "ouvir" a porta 8080 de repente passasse a ser o canal exclusivo de troca de dados com o "Cliente A", ele ficaria "ocupado". Enquanto o Cliente A estivesse conectado, nenhum outro cliente conseguiria se conectar, pois não haveria mais ninguém escutando a porta 8080 para receber novos pedidos de conexão. O servidor atenderia apenas um cliente por vez.

Para resolver isso e permitir múltiplas conexões simultâneas (concorrência), o sistema operacional divide as responsabilidades em dois tipos de sockets:

* **Socket de Escuta (Listener Socket):** O socket original. Ele fica estático, no estado `LISTEN`. Sua única função é receber requisições de conexão (pacotes SYN do TCP) e colocá-las em uma fila.
* **Socket Conectado (Connection Socket):** Quando o sistema chama `accept()`, ele retira a primeira requisição dessa fila e **clona** as configurações originais para criar um **novo** File Descriptor (FD). Esse novo socket entra no estado `ESTABLISHED` e é dedicado exclusivamente a conversar com aquele cliente específico.

O socket original (o *listener*) continua intacto, livre e no estado `LISTEN`, pronto para aceitar o "Cliente B", "Cliente C" e assim por diante.

### 2. A Prova no Seu Código Go

O código que você forneceu foi desenhado perfeitamente para provar exatamente esse conceito, extraindo e imprimindo os *File Descriptors* (FDs) por baixo dos panos. O File Descriptor é o "número de identidade" que o sistema operacional (Linux/Mac/Windows) dá para cada arquivo ou conexão de rede aberta.

Vamos analisar como o código evidencia a teoria:

**A Criação do "Recepcionista" (Listener):**

```go
listener, err := net.Listen("tcp", ":8080")
// ...
listenerFD := rawFD(listener.(*net.TCPListener))

```

Aqui o código cria o socket de escuta e guarda o FD dele na variável `listenerFD`. Vamos supor que o sistema operacional dê a ele o **FD 3**.

**O Loop de Atendimento (`accept`):**

```go
for {
    conn, err := listener.Accept() // Fica bloqueado aqui até um cliente chegar
    // ...
    connFD := rawFD(conn.(*net.TCPConn))

```

Quando o comando `nc localhost 8080` é executado no terminal, o `Accept()` acorda e retorna a variável `conn`. O código então extrai o FD dessa nova conexão (`connFD`).

* `conn`: **connected socket** novo criado pelo kernel após handshake TCP — FD próprio, 4-tupla completa preenchida (LocalAddr + RemoteAddr)

**A Revelação no Terminal:**

```go
fmt.Printf("  Conn FD:     %d  <- socket dessa conexão\n", connFD)
fmt.Printf("  Listener FD: %d  <- continua o mesmo\n", listenerFD)

```

Se você rodar esse código e conectar 3 clientes, verá algo parecido com isto no log:

* **Startup:** Listener FD: 3
* **Cliente 1:** Conn FD: 4 | Listener FD: 3
* **Cliente 2:** Conn FD: 5 | Listener FD: 3
* **Cliente 3:** Conn FD: 6 | Listener FD: 3

O `Listener FD` nunca muda. Ele continua sendo o número 3, apenas monitorando a porta 8080. A cada `Accept()`, o sistema operacional entrega um novo recurso (FD 4, depois 5, depois 6...), permitindo que você passe essa nova conexão para uma Goroutine (como feito no final do seu código: `go func(c net.Conn)`) para que os dados sejam lidos em paralelo, sem bloquear o socket principal.

### 3. A Analogia da Recepção de Hotel

Para fixar a explicação de forma didática:

1. O **Socket Listener (FD 3)** é o recepcionista do hotel, que fica parado na porta da frente (Porta 8080).
2. Quando um cliente chega (requisição de conexão), o recepcionista diz: *"Bem-vindo! Vou pedir para um mensageiro acompanhá-lo até a sua sala privada para fazer o check-in."*
3. A função **`accept()`** é o ato de chamar esse mensageiro e criar essa sala privada (**Novo Socket / FD 4**).
4. O cliente e o mensageiro vão para a sala privada trocar informações (troca de pacotes TCP).
5. O recepcionista (FD 3) **não sai do lugar**. Ele continua na porta da frente, no mesmo estado, pronto para receber o próximo cliente.

---

### 4. Observação prática: a "lacuna" de FDs e o netpoller

Ao rodar o lab e inspecionar `/proc/<pid>/fd/` durante 3 conexões, a sequência **não foi contínua**. Em vez de `4, 5, 6`, observou-se:

```
FD 3 -> socket:[6942]            (listener)
FD 4 -> socket:[7002]            (conn #1)
FD 5 -> anon_inode:[eventpoll]   (netpoller)
FD 6 -> anon_inode:[eventfd]     (netpoller)
FD 7 -> socket:[7656]            (conn #2)
FD 8 -> socket:[7079]            (conn #3)
```

Os FDs 5 e 6 **não são sockets** — são artefatos do runtime do Go, criados implicitamente pelo netpoller:

| FD | Tipo | Função |
|----|------|--------|
| 5 | `anon_inode:[eventpoll]` | Instância do `epoll` do kernel (`epoll_create1`). Onde o netpoller registra todo socket Go via `epoll_ctl(EPOLL_CTL_ADD)`. |
| 6 | `anon_inode:[eventfd]` | `eventfd()`. Truque para "interromper" `epoll_wait` quando o runtime precisa acordar a thread monitora (novo FD a registrar, timer expirando). Write nesse fd → epoll retorna → thread reconfigura → volta a esperar. |

#### Por que aparecem entre conn #1 e conn #2 e não no startup?

O runtime do Go cria o epoll **lazy** — só na primeira vez que alguma goroutine precisa ser parkada em I/O. `net.Listen` sozinho não dispara. `Accept` (que bloqueia a goroutine principal esperando handshake) dispara. O momento exato em que os FDs 5 e 6 nascem depende de qual syscall primeiro força o runtime a registrar algo no epoll.

#### Por que isso importa pedagogicamente

Prova que **o netpoller é infraestrutura do runtime, não do código do usuário**. Em momento nenhum o `main.go` chamou `epoll_create` ou `eventfd` — só fez `net.Listen`, `Accept`, `go func` e `Read`. Toda a maquinaria de I/O multiplexado nasceu por baixo dos panos.

Ou seja: quando lemos "Go esconde `socket()+bind()+listen()`" no `GUIDE.md`, **também esconde `epoll_create1()` e `eventfd()`**. A "lacuna" nos FDs é a primeira evidência observável dessa camada invisível.

#### Como reproduzir

Dentro do container, antes do 1º `Accept`:
```bash
ls -la /proc/<pid>/fd/   # só verá FDs 0-3
```
Depois de `nc localhost 8080` (1ª conexão) → repete o `ls` → FDs 5 e 6 aparecem do nada.