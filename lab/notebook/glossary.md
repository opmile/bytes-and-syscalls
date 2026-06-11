# Glossário de Incorporação

## Processos vs Threads

Processo é um programa em execução com memória isolada. Thread é uma linha de execução dentro de um processo que compartilha a mesma memória.
Aqui está o resumo essencial:

* Espaço de Memória: Processos são isolados; threads compartilham dados do mesmo processo.
* Consumo de Recursos: Criar processos é pesado; criar threads é leve e rápido.
* Segurança e Estabilidade: Se um processo cai, o sistema continua; se uma thread trava, o processo inteiro pode cair.
* Comunicação: Processos dependem de mecanismos complexos; threads conversam diretamente pela memória compartilhada.

Para te ajudar a fixar o conceito, você precisa desse resumo para estudar para um concurso/prova ou para aplicar em um projeto de programação?

## Scheduler do Kernel

O escalonamento do kernel (process scheduling) é o mecanismo do sistema operacional que decide qual processo utilizará a CPU, por quanto tempo e quando será interrompido. Seu objetivo é equilibrar a taxa de transferência (throughput), minimizar o tempo de resposta (latency) e garantir a máxima eficiência.

Schedulers are like the traffic police of the kernel. The Linux kernel has several schedulers, each with its strengths and weaknesses. The default scheduler is the Completely Fair Scheduler, which is designed to provide fair CPU time to all processes. However, other schedulers are optimized for specific use cases, such as the Early Deadline First Scheduler and the Real-Time Scheduler, as well many others with different use cases.

https://documentation.ubuntu.com/real-time/latest/explanation/schedulers/

## Semáforo 

Um semáforo em programação concorrente é uma variável especial usada para controlar o acesso a recursos compartilhados e coordenar a execução de múltiplas threads ou processos. Ele funciona como um "pedágio" ou contador de permissões, evitando que vários processos entrem em conflito ao modificar os mesmos dados

In Go, a semaphore is a synchronization tool used to control how many goroutines can access a shared resource or run concurrently at the same time

* Backpressure in the Linux kernel refers to the systemic mechanisms used to slow down data flow when a receiver, buffer, or disk cannot process data as fast as it is being sent