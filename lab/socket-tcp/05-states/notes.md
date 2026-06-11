# Ciclo de vida de uma conexão TCP: handshakes e estados

Guia unificado que junta a mecânica dos handshakes TCP com os estados reais
expostos pelo sistema operacional, conectando a teoria do protocolo ao
cotidiano de quem programa ou opera infraestrutura.

Para entender `TIME_WAIT` e `CLOSE_WAIT`, modele a conexão TCP como uma
conversa formal dividida em três atos: abertura, transmissão e encerramento.

```
[Ato 1: Abertura]  --->  [Ato 2: Transmissão]  --->  [Ato 3: Encerramento]
 (3-Way Handshake)         (Dados Fluindo)             (4-Way Handshake)
```

## Ato 1: abertura (three-way handshake)

Nenhum dado trafega antes de cliente e servidor apertarem as mãos três vezes.
O objetivo é sincronizar os dois lados e garantir que ambos conseguem enviar e
receber.

1. **SYN** (cliente → servidor): o cliente envia a intenção de conexão.
   "Quero conversar. Meu número de partida é X."
2. **SYN-ACK** (servidor → cliente): o servidor aceita e devolve o seu.
   "Topo. Recebi seu X (devolvo ACK X+1) e meu número de partida é Y."
3. **ACK** (cliente → servidor): o cliente finaliza o acordo.
   "Recebi seu Y (devolvo ACK Y+1)."

A partir desse momento, o status exibido por `netstat` ou `ss` muda para
`ESTABLISHED`. Os dados passam a trafegar livremente (Ato 2).

## Ato 3: encerramento (four-way handshake e os estados)

O TCP é full-duplex: as vias de ida e volta são independentes. Por isso fechar
a conexão exige quatro etapas. Assumindo que o cliente iniciou o fechamento:

```
      CLIENTE (Iniciador)                         SERVIDOR (Receptor)

        ESTABLISHED                                 ESTABLISHED
             |                                           |
  (1) FIN    |------------------------------------------>|
             |                                           |
        FIN_WAIT_1                                  CLOSE_WAIT (aplicação avisada)
             |                                           |
  (2) ACK    |<------------------------------------------|
             |                                           |
        FIN_WAIT_2                                       |  (servidor termina de processar
             |                                           |   e libera recursos locais)
             |                                           |
  (3) FIN    |<------------------------------------------|
             |                                           |
        TIME_WAIT                                    LAST_ACK
             |                                           |
  (4) ACK    |------------------------------------------>|
             |                                           |
      (Espera 2*MSL)                                  CLOSED
             |
          CLOSED
```

### Passo 1: o cliente pede para sair

O cliente envia um `FIN`.

- **Significado:** "Não tenho mais nada para enviar."
- **Estado:** o cliente entra em `FIN_WAIT_1`.

### Passo 2: o servidor aceita, e nasce o CLOSE_WAIT

O servidor recebe o `FIN` e responde com um `ACK`.

- **Significado:** "Entendido. Não espero mais dados de você."
- **Estado no servidor (`CLOSE_WAIT`):** o servidor entra imediatamente em
  `CLOSE_WAIT`. O sistema operacional avisa a aplicação (o código Java, Python,
  Node etc.): a outra ponta fechou a conexão; feche a sua parte.

A conexão fica meio-fechada: o cliente não pode mais enviar dados, mas o
servidor ainda pode terminar de enviar o que estava no buffer.

### Passo 3: o servidor se despede

A aplicação do servidor termina o processamento e executa o fechamento do
socket (`socket.close()`). O servidor envia o seu próprio `FIN`.

- **Significado:** "Também terminei. Estou fechando minha via de envio."
- **Estado:** o servidor muda de `CLOSE_WAIT` para `LAST_ACK`.

### Passo 4: o cliente confirma, e nasce o TIME_WAIT

O cliente recebe o `FIN` do servidor e responde com o último `ACK`.

- **Significado:** "Recebido. Acabou."
- **Estado no cliente (`TIME_WAIT`):** o servidor recebe esse `ACK` e vai para
  `CLOSED` imediatamente. O cliente não pode sumir da rede na mesma hora: entra
  em `TIME_WAIT`.

## Diagnóstico: saudável vs. doente

### TIME_WAIT: mecanismo de segurança (saudável)

`TIME_WAIT` aparece sempre no **lado que inicia o fechamento** (quem envia o
primeiro `FIN`, o *active close*). No exemplo acima é o cliente, mas não é uma
regra fixa de cliente: se o servidor fechar primeiro, é ele que entra em
`TIME_WAIT`. O critério é quem fecha ativamente, não o papel na conexão.

O lado ativo fica em `TIME_WAIT` por cerca de 1 a 2 minutos, por zelo
protocolar:

1. **Garantia:** se o último `ACK` (passo 4) se perder, o servidor retransmite
   o `FIN` (passo 3). O cliente precisa estar vivo em `TIME_WAIT` para reenviar
   o `ACK`.
2. **Segurança:** evita que pacotes atrasados da conexão antiga vaguem pela
   rede e entrem como intrusos em uma nova conexão que reutilize as mesmas
   portas e IPs.

Diagnóstico: é gerenciado pelo kernel e some sozinho. Sinal de que o protocolo
funcionou corretamente.

### CLOSE_WAIT: vazamento no código (sinal de bug)

Centenas ou milhares de conexões travadas em `CLOSE_WAIT` no servidor indicam
bug na aplicação, não na rede.

O fluxo travou no passo 2: o servidor recebeu o `FIN`, o SO colocou o socket em
`CLOSE_WAIT` e avisou a aplicação, mas a aplicação nunca executou o passo 3.

Não confunda `CLOSE_WAIT` com `CLOSED`:

- `CLOSE_WAIT` é um estado **intermediário**, à espera de uma ação da
  aplicação. O SO já recebeu o `FIN` do par e está aguardando o seu
  `close()` (passo 3). A conexão está meio-fechada: o par não envia mais dados,
  mas você ainda pode. O socket e o file descriptor continuam alocados.
- `CLOSED` é o estado **final**. O four-way handshake terminou, o socket foi
  liberado e o file descriptor devolvido ao SO. Nada pendente.

A diferença prática: `CLOSE_WAIT` espera que **a sua aplicação** aja;
`CLOSED` significa que acabou.

Causas comuns:

- Faltou `connection.close()` ou `response.body.close()`.
- A thread que cuidava da conexão lançou uma exceção antes da linha de
  fechamento.
- A aplicação entrou em deadlock.

Perigo: como a aplicação nunca fecha o socket, o servidor esgota seus file
descriptors (limite de arquivos/sockets abertos no Linux). No limite, o serviço
para de atender e dispara o erro `Too many open files`.

### Resumo: três perguntas-âncora

| Pergunta | Resposta |
|---|---|
| `TIME_WAIT` aparece em qual lado? | No lado que fecha ativamente (envia o primeiro `FIN`). Geralmente o cliente, mas é o *active close* que define, não o papel. |
| `CLOSE_WAIT` vs `CLOSED`? | `CLOSE_WAIT` é intermediário, à espera do `close()` da aplicação (passo 3 pendente, FD ainda alocado). `CLOSED` é final: handshake completo, socket liberado. |
| Qual é saudável e qual é bug? | `TIME_WAIT` é saudável — o kernel o gerencia e ele some sozinho. `CLOSE_WAIT` acumulado é bug — a aplicação esqueceu de fechar e vaza file descriptors. |

Mnemônico: `TIME_WAIT` é problema do kernel (resolve sozinho); `CLOSE_WAIT` é
problema seu (o código travou no passo 3).

## defer `conn.Close()` em Go

O `defer conn.Close()` é a principal defesa do ecossistema Go contra o acúmulo
de conexões em `CLOSE_WAIT`. É o mecanismo que garante a execução do passo 3 do
four-way handshake independentemente do que ocorra dentro da função.

### O que o defer faz

`defer` antes de uma chamada instrui o compilador: adie esta execução para o
último instante possível. Quando a função terminar — por chegar ao fim, por um
`return` ou por um `panic` —, execute isto.

```go
func processRequest() {
    // 1. Abre a conexão (3-way handshake) -> ESTABLISHED
    conn, err := net.Dial("tcp", "google.com:80")
    if err != nil {
        return
    }

    // 2. Garante a limpeza futura
    defer conn.Close()

    // 3. Daqui para baixo transmite dados (Ato 2).
    // Erro, return ou panic: o Go garante que conn.Close() roda na saída.
}
```

### Paralelo com os estados TCP

| Momento no código Go | Protocolo TCP | Estado |
|---|---|---|
| `net.Dial(...)` | Envia SYN, recebe SYN-ACK, envia ACK (3-way) | `ESTABLISHED` |
| O outro lado decide fechar | SO recebe o FIN e avisa o Go | `CLOSE_WAIT` |
| A função chega ao fim (ou dá erro) | `defer conn.Close()` é disparado pelo runtime | aplicação fecha o socket |
| `conn.Close()` roda | Go envia o FIN de volta (passo 3) | `CLOSE_WAIT` → `LAST_ACK` |
| O outro lado responde com ACK | Fim do four-way handshake | `CLOSED` |

### Esquecer o defer: o nascimento do bug

```go
func processWithBug() {
    conn, _ := net.Dial("tcp", "api.interna:8080")

    // ... sem defer conn.Close() ...

    err := doSomething(conn)
    if err != nil {
        log.Println("Erro ao processar dados")
        return // a função morre aqui e o socket fica aberto
    }

    conn.Close() // legítimo, mas inalcançável se houver erro acima
}
```

Sequência do desastre:

1. `doSomething(conn)` falha e o código cai no `return`.
2. A função encerra e a variável `conn` sai de escopo.
3. A `api.interna`, ao perceber o fim do processo, envia um `FIN`.
4. O SO do servidor recebe o `FIN`, coloca a conexão em `CLOSE_WAIT` e sinaliza
   a aplicação Go.
5. A aplicação não tem mais a referência de `conn` (perdida no `return`). O
   `conn.Close()` nunca poderá ser chamado.

Resultado: a conexão fica travada em `CLOSE_WAIT` até o serviço reiniciar. Se a
função for chamada milhares de vezes por minuto, o servidor trava por
esgotamento de file descriptors.

### Por que o Go incentiva o defer

Linguagens antigas sofriam com acúmulo de `CLOSE_WAIT` porque o fechamento
ficava distante da abertura no código, ou dependia de blocos
`try-catch-finally` complexos. O `defer` mantém abertura e regra de fechamento
visualmente coladas: abriu, garanta o fechamento na linha de baixo. Essa
simplicidade evita que a infraestrutura caia por um deslize no tratamento de
erro.
