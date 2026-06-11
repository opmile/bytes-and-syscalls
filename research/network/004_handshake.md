# Three-way handshake TCP

O handshake serve para **sincronizar os sequence numbers iniciais** (ISN) de cada lado e confirmar que ambos conseguem enviar e receber. TCP é full-duplex, então cada direção tem seu próprio fluxo de bytes numerado independentemente — e cada lado precisa saber por qual número o outro vai começar a contar.

## A troca

**SYN** (cliente → servidor)
Cliente envia segmento com flag SYN ligada e seu sequence number inicial, `ISN_c` (escolhido aleatoriamente para evitar colisão com conexões antigas e mitigar spoofing). Significado: "quero abrir conexão, vou começar a numerar meus bytes a partir de ISN_c".

**SYN-ACK** (servidor → cliente)
Servidor responde com SYN + ACK no mesmo segmento. Carrega:
- Seu próprio `ISN_s` (numeração da direção servidor→cliente)
- `ACK = ISN_c + 1`, confirmando o SYN recebido

Significado: "aceito, vou numerar a partir de ISN_s, e confirmo que recebi seu SYN".

**ACK** (cliente → servidor)
Cliente confirma o SYN do servidor com `ACK = ISN_s + 1`. A conexão entra em estado ESTABLISHED dos dois lados e pode começar a trafegar dados.

## Por que três passos, não dois

Dois passos confirmariam só uma direção. O terceiro ACK fecha a simetria: o servidor precisa saber que o cliente recebeu seu ISN_s, senão poderia enviar dados que o cliente rejeitaria como fora de sequência. Três é o mínimo para que **ambos os lados tenham certeza de que o outro está pronto e sincronizado**.

## Custo

Um RTT completo antes do primeiro byte de payload — é a origem do "1 RTT de overhead" que justifica connection pooling. Em TLS, soma-se mais 1-2 RTTs do handshake criptográfico em cima desse.