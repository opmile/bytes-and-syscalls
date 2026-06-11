# Throughput — o essencial

**O que é**

Throughput é o volume de dados que efetivamente passa pela rede num intervalo de tempo. Latência é o atraso de uma viagem; throughput é a vazão. Mede-se em bytes/segundo (KBps, MBps, GBps).

Analogia útil: latência é quanto tempo um carro leva para atravessar uma rodovia; throughput é quantos carros chegam ao destino por hora.

**Throughput vs largura de banda**

Largura de banda é o teto teórico (capacidade máxima do canal). Throughput é o que você realmente consegue trafegar dadas as condições reais: perda de pacotes, congestionamento, hardware. Largura de banda alta não garante throughput alto, mas limita o teto.

**O que afeta throughput**

- Largura de banda (limite superior absoluto)
- Capacidade de processamento dos dispositivos da rede
- Perda de pacotes (retransmissão derruba a vazão útil)
- Topologia da rede (caminhos disponíveis, gargalos)

**Relação com latência**

As duas se influenciam. Alta latência tende a reduzir throughput porque cada pacote demora mais para ser confirmado (especialmente em TCP, que espera ACKs). Baixa throughput faz a rede *parecer* lenta mesmo se cada pacote individualmente chega rápido.

**Trade-off de protocolo (importante)**

- **TCP**: confirma entrega, retransmite perdidos. Maior latência, maior throughput confiável. Bom para transferência de dados.
- **UDP**: não verifica entrega, dispara e esquece. Menor latência, throughput cru maior, mas perde pacotes. Bom para streaming e jogos onde um frame perdido importa menos que o próximo chegar rápido.

**Como pensar isso em sistema**

A virada de chave em relação ao RTT: **RTT é uma métrica por interação; throughput é uma métrica de capacidade agregada**. Você otimiza RTT para um usuário individual ter resposta rápida. Você otimiza throughput para que *muitos usuários simultâneos* sejam atendidos sem que o sistema afunile.

Implicações de arquitetura:

- **Latência baixa não implica throughput alto**. Um endpoint pode responder em 50ms para 1 usuário e cair para 5 segundos com 1000 usuários simultâneos se algum recurso compartilhado (DB connection pool, fila, CPU) saturar. Você só descobre isso com load test, não com curl.

- **Throughput é limitado pelo recurso mais escasso na cadeia**. Pode ser CPU do app, IOPS do disco, conexões disponíveis do DB, banda de saída. Otimizar qualquer outra coisa antes de identificar o gargalo real é trabalho desperdiçado.

- **Cache e CDN melhoram os dois ao mesmo tempo**. Reduzem RTT (resposta vem de mais perto) e aumentam throughput agregado (origem fica livre para atender o que não está em cache).

- **Escolha de protocolo é decisão de produto**. Streaming de vídeo, voz, telemetria de IoT — UDP. Transferência de arquivo, transação financeira — TCP. Não é detalhe de infra, é alinhamento entre requisitos e ferramenta.

A regra prática: ao desenhar um sistema, pergunte separadamente "qual o RTT alvo por request?" e "quantos requests por segundo precisamos sustentar?". As respostas levam a otimizações diferentes — às vezes opostas. Adicionar uma fila, por exemplo, *aumenta* RTT individual mas *protege* throughput sob picos.

**O ponto de partida**

RTT individual e throughput agregado **não são a mesma métrica e às vezes puxam para lados opostos**. Você pode otimizar um piorando o outro.

**O caso da fila**

Imagina que seu sistema recebe 1000 requests/segundo num pico, mas seu app só consegue processar 500/segundo.

**Sem fila:**

- Os 500 que cabem: respondem em 50ms (RTT ótimo)
- Os outros 500: o servidor lota, começa a recusar conexões ou travar
- Throughput real: cai para muito menos que 500/s porque o sistema entra em colapso

**Com fila (tipo SQS, RabbitMQ):**

- Todos os 1000 requests entram numa fila e recebem "ok, recebi"
- O app processa no seu ritmo (500/s) consumindo da fila
- RTT individual de cada request: pior, porque agora ele espera na fila antes de ser processado (pode ser 50ms, 200ms, 2s dependendo do tamanho da fila)
- Throughput: estável e previsível, ninguém é recusado

**A conexão do trade-off**

Você trocou **resposta rápida por request** por **capacidade de absorver volume sem quebrar**. Cada request individual demora mais, mas o sistema como um todo atende mais gente sem cair.

É por isso que filas aparecem em sistemas que processam pagamento, envio de email, processamento de vídeo: ninguém precisa do resultado em 50ms, mas todo mundo precisa que o request *não se perca* num pico.