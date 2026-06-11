## Socket TCP/IP

Um socket é um **endpoint de comunicação** — a abstração que o SO expõe para que um processo envie e receba dados pela rede.

* é o ponto final de comunicação bidirecional entre processos em uma rede

Conceitualmente: se TCP/IP é o protocolo que define *como* os dados trafegam, o socket é a *interface* que seu código usa para acessar esse protocolo

* permite a troca de dados (leitura/escrita) entre um cliente e um servidor. -> i/o operation 

---

**O que identifica um socket**

Um socket é identificado pela tupla:

```
(IP origem, porta origem, IP destino, porta destino, protocolo)
```

Duas conexões TCP distintas ao mesmo servidor podem existir simultaneamente porque as portas de origem diferem.

---

**Como funciona na prática**

```
servidor                          cliente
socket()                          socket()
bind(IP, porta)
listen()
                                  connect(IP_servidor, porta)
accept()  <--- TCP handshake --->
// retorna novo socket             // conexão estabelecida
read()/write()  <--- dados --->   read()/write()
close()                           close()
```

`accept()` é importante: o servidor tem um socket que *escuta* e, para cada cliente conectado, cria um socket separado. Por isso um servidor pode atender N clientes simultaneamente.

---

**Portas**

Porta é um número (0–65535) que identifica *qual processo* no host deve receber o pacote. O IP roteia até a máquina; a porta roteia até o processo.

Convenções: HTTP = 80, HTTPS = 443, SSH = 22. Portas < 1024 requerem privilégio de root.

---

**Socket vs conexão**

- Socket TCP é orientado a conexão — estado é mantido nos dois lados (buffers, número de sequência, estado da janela).
- UDP socket não tem conexão — você só envia datagramas, sem handshake, sem estado.

---

Esse é o modelo que linguagens de alto nível (Go's `net.Dial`, Java's `Socket`, etc.) encapsulam. Quando você faz `http.Get()` em Go, por baixo existe um socket TCP sendo aberto, o handshake acontecendo, e os bytes do HTTP trafegando por ele.

---

### I/O + Socets TCP 

**O que acontece num `write(socket, dados)`**

```
seu processo
  → write() copia dados pro send buffer (kernel)
  → retorna imediatamente*
  → TCP pega do buffer, segmenta, envia
  → aguarda ACK do receiver
  → libera espaço no buffer
```

Você não escreve direto na rede. Escreve no **buffer do kernel**, e o TCP cuida do resto de forma assíncrona.

---

**O link com I/O bound**

O `write()` em si raramente bloqueia — só bloqueia se o **send buffer estiver cheio**, o que acontece quando:

- o receiver está lento para consumir (TCP flow control)
- a rede está congestionada (TCP congestion control)

Quando bloqueia, sua thread/goroutine fica parada esperando espaço no buffer. I/O bound.

O `read()` é mais frequentemente o gargalo — bloqueia até dados chegarem no **receive buffer**.

---

**Resumo**

| Operação | Bloqueia quando |
|----------|----------------|
| `write()` | send buffer cheio |
| `read()` | receive buffer vazio |

Ambos são operações sobre buffers do kernel, não acesso direto à rede. O TCP opera entre esses buffers e o fio.

---

### From TCP to HTTP

Construir um servidor HTTP/1.1 do zero 

> https://www.youtube.com/watch?v=FknTw9bJsXM&t=79

O que está sendo feito é exatamente a camada entre TCP e HTTP: você abre um socket TCP, aceita conexões brutas, e lê os bytes que chegam no receive buffer. Esses bytes são texto puro seguindo o formato HTTP/1.1 — `GET / HTTP/1.1\r\nHost: ...\r\n\r\n` — e o que você constrói é o parser que interpreta esse texto e o construtor que monta a resposta no mesmo formato antes de escrever de volta no socket. Em Go isso fica explícito porque `net.Listen` te dá o socket, `conn.Read()` te dá os bytes crus, e tudo entre isso e "entender que é um GET para `/`" é o que você implementa à mão — o que frameworks como `net/http` abstraem completamente. A lição é que HTTP não é mágica de protocolo: é uma convenção de formatação de texto em cima de uma stream TCP confiável.

Para aprendizado o benefício é real e denso: você para de tratar HTTP como caixa preta e passa a entender por que headers têm `\r\n`, por que `Content-Length` existe, por que keep-alive importa para performance, e onde frameworks como `net/http` estão te poupando trabalho — o que te torna mais capaz de debugar, otimizar e tomar decisões arquiteturais. Em produção, implementar do zero seria overengineering quase certo, mas o conhecimento adquirido não é — ele aparece quando você precisa tunar timeouts, entender um bug de conexão, ou avaliar se HTTP/2 resolve seu problema.

---

### `*http.Client` mantém um pool de conexões tcp, criar um novo a cada chamada desperdiça handshakes.

O pool é exatamente o que evita recriar o que discutimos a cada request: TCP handshake (SYN/SYN-ACK/ACK) + TLS handshake custam RTTs reais, e cada novo `http.Client` joga fora conexões que já passaram por isso. O pool mantém sockets TCP abertos e ociosos no estado `ESTABLISHED` — quando chega um novo request, o `http.Client` pega uma conexão do pool, já com buffer de kernel alocado e sequência TCP estabelecida, e vai direto pro `write()` com o HTTP request. A relação com I/O bound fecha aqui: sem pool, parte do tempo bloqueado que você atribui a "espera de rede" é na verdade overhead de handshake evitável — o custo não é só latência de transferência de dados, é latência de setup de conexão repetido desnecessariamente.

---

### I/O Bound 

Aplicações I/O bound têm seu gargalo em espera — e handshakes são espera pura sem nenhum trabalho útil de CPU. O pool elimina essa espera recorrente, reduzindo o tempo total bloqueado em I/O sem mudar nada no processamento. A distinção prática: se você profila uma aplicação que cria `http.Client` a cada chamada e vê CPU ociosa com threads bloqueadas, o problema não é CPU bound (não falta poder de processamento) nem necessariamente lógica lenta — é I/O bound por setup de conexão desnecessário. Adicionar goroutines não resolve; reusar conexões sim.

**Comparando Maquinários**

O ponto de convergência é exatamente onde cada modelo bloqueia num `read()` de socket.

**Tomcat** aloca uma thread de SO por conexão. Quando essa thread bloqueia esperando dados no receive buffer, ela fica suspensa consumindo ~1MB de stack. O pool de threads existe pelo mesmo motivo que o pool de conexões TCP — criar e destruir é caro. O gargalo de I/O bound aqui é multiplicado: você tem threads de SO ociosas, cada uma presa num socket esperando.

**JavaScript** nunca bloqueia uma thread — o event loop usa `epoll`/`kqueue` por baixo, monitorando N sockets simultaneamente. Quando dados chegam no receive buffer de algum socket, o evento dispara o callback. Você escreve `await fetch()` e está explicitamente cedendo controle ao loop. Single-thread, então CPU bound real trava tudo.

**Go** escreve igual ao Tomcat — `conn.Read()` parece bloqueante — mas quando a goroutine bloqueia num socket, o runtime a tira da thread de SO e coloca outra goroutine no lugar, igual ao event loop do JS, só que invisível pra você. Por baixo o runtime também usa `epoll`. A diferença é que o scheduler faz o trabalho do `await` implicitamente.

O fechamento com o pool de conexões: nos três modelos, reusar conexões TCP estabelecidas reduz I/O bound por handshake. Mas em Go e JS o custo de ter muitas conexões abertas e ociosas é menor — goroutines e event loop lidam com sockets ociosos quase de graça, enquanto no Tomcat cada conexão idle ainda amarra uma thread de SO.