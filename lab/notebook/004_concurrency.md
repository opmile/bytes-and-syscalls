O runtime do Go bloqueia a **Goroutine**, e não a Thread do Sistema Operacional.

Embora o kernel e o Netpoller usem O_NONBLOCK e epoll para nunca deixar a Thread (Thread M) ociosa, a API que o Go te entrega é **síncrona para a Goroutine**. Se uma Goroutine chama uma operação que não está pronta, *aquela Goroutine específica* é pausada (parkada) pelo scheduler do Go.

Para entender o impacto disso, imagine o fluxo do seu código se você **não** usasse a Goroutine para o Read:

### Cenário SEM go func (Sequencial)

1. A **Goroutine Principal** executa listener.Accept() e fica pausada até o **Cliente 1** conectar.
2. O Cliente 1 conecta. A Goroutine Principal acorda e avança para a próxima linha: conn.Read(buf).
3. Não há dados prontos do Cliente 1 ainda. O Netpoller intercepta o EAGAIN do kernel e **pausa a Goroutine Principal**.
4. **O problema:** Como a Goroutine Principal está pausada esperando o Read do Cliente 1, ela **não consegue voltar para o topo do loop for** para chamar o Accept() novamente.

> ❌ **Resultado:** Enquanto o Cliente 1 não enviar nenhum dado, seu servidor fica completamente "surdo". Um Cliente 2 tentará conectar e ficará preso no handshake do kernel, porque ninguém está chamando Accept() no user space.

---

### Cenário COM go func (Concorrente)

1. A **Goroutine Principal** executa listener.Accept() e fica pausada até o **Cliente 1** conectar.
2. O Cliente 1 conecta. A Goroutine Principal acorda e executa go func(...), o que joga o socket do Cliente 1 para uma **Nova Goroutine (Goroutine B)**.
3. A Goroutine Principal ignora o Read e **volta imediatamente para o topo do loop for**, chamando Accept() de novo. Ela volta a vigiar novas conexões.
4. Enquanto isso, na **Goroutine B**, o Read é chamado. Se não houver dados, o Netpoller pausa apenas a **Goroutine B**.

> **Resultado:** A Goroutine Principal continua livre e rodando em loop no Accept(), permitindo que o Cliente 2, 3 e 4 entrem no servidor, cada um ganhando sua própria Goroutine isolada para executar o seu respectivo Read.

### Resumo

O mecanismo de I/O Multiplexado (epoll) garante que o Go precise de pouquíssimas **Threads do sistema operacional** para gerenciar a espera. Porém, para que o seu fluxo lógico consiga esperar por coisas diferentes ao mesmo tempo (esperar um cliente novo no Accept E esperar dados de um cliente antigo no Read), você ainda precisa separar essas tarefas em **linhas de execução lógicas distintas**, que são as Goroutines.