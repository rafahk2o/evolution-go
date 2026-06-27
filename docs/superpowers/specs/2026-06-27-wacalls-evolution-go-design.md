# Integracao WaCalls na Evolution Go

## Objetivo

Adicionar chamadas de voz WhatsApp completas a Evolution Go, reutilizando a conexao `whatsmeow` de cada instancia. A entrega deve suportar chamadas de entrada e saida, audio bidirecional no navegador por WebRTC, controle concorrente por operador e eventos consumiveis por clientes futuros, incluindo um widget do Chatwoot.

## Escopo

Esta especificacao inclui:

- inicio, aceite, rejeicao e encerramento de chamadas;
- sinalizacao de chamadas do WhatsApp;
- transporte de midia MLow/SRTP;
- ponte WebRTC com audio PCM;
- estado e ownership de chamadas em memoria;
- API HTTP autenticada e stream SSE;
- eventos normalizados e compatibilidade com o endpoint de rejeicao atual;
- testes unitarios, HTTP, concorrentes e de integracao de midia.

Esta especificacao nao inclui:

- widget ou alteracoes no Chatwoot;
- tokens temporarios para navegadores;
- gravacao de chamadas;
- conferencia ou chamadas em grupo;
- persistencia de historico no banco;
- recuperacao de chamadas apos reinicio do processo.

## Decisao de arquitetura

O motor de chamadas do WaCalls sera migrado para pacotes internos da Evolution Go e adaptado para usar o `whatsmeow.Client` que ja pertence a instancia. Nao sera iniciado um segundo cliente WhatsApp e nao havera um sidecar independente.

Essa opcao reduz conflitos de sessao, elimina sincronizacao de credenciais entre processos e permite que a chamada siga o mesmo ciclo de conexao, autenticacao, logs e webhooks das demais operacoes da instancia.

O codigo derivado do WaCalls, licenciado sob MIT, manterá a atribuicao exigida. Dependencias e trechos importados terao seus avisos de licenca preservados no repositorio.

## Componentes

### CallRegistry

Mantem os gerenciadores por instancia e os indices de chamadas ativas. E responsavel por busca, registro, claim atomico, remocao e limpeza completa quando a instancia desconecta.

### CallManager

Implementa a maquina de estados de uma chamada, valida transicoes, controla o `clientId` proprietario e coordena sinalizacao, midia e publicacao de eventos. Uma chamada terminal nao pode voltar a um estado ativo.

### WhatsAppAdapter

Adapta o `whatsmeow.Client` existente para a interface exigida pelo motor WaCalls. Encaminha ofertas, aceite, transporte, rejeicao e termino recebidos pelo handler global de eventos da Evolution Go.

### Signaling

Contem o protocolo de estabelecimento e encerramento de chamada migrado do WaCalls. Nao conhece HTTP, Gin, instancias persistidas ou Chatwoot.

### MediaPipeline

Processa pacotes MLow e SRTP entre a Evolution Go e o relay do WhatsApp. O pipeline deve poder ser testado sem conexao real por meio de interfaces de transporte.

### WebRTCBridge

Cria a conexao WebRTC e um data channel por chamada. O navegador envia e recebe PCM signed 16-bit little-endian, mono, 16 kHz. A ponte converte os frames entre PCM e o pipeline do WhatsApp e encerra seus recursos quando a chamada termina.

### EventBroker

Distribui eventos normalizados para assinantes SSE e para o mecanismo de webhooks existente. Consumidores lentos nao podem bloquear a sinalizacao ou o audio; seus buffers serao limitados e a conexao sera encerrada quando nao acompanhar o stream.

### CallHandler

Expoe os contratos HTTP, usa a instancia colocada no contexto pelo middleware atual e converte erros de dominio em respostas consistentes.

## API HTTP

Todas as rotas usam o middleware de autenticacao atual da Evolution Go. O header `X-Call-Client-ID` identifica o operador ou navegador para ownership e concorrencia, mas nao substitui autenticacao.

### Iniciar chamada

`POST /call/start`

```json
{
  "number": "5511999999999"
}
```

Resposta `201 Created`:

```json
{
  "callId": "call-id",
  "direction": "outgoing",
  "status": "starting"
}
```

### Negociar WebRTC

`POST /call/:callId/webrtc`

```json
{
  "sdpOffer": "v=0..."
}
```

Resposta `200 OK`:

```json
{
  "sdpAnswer": "v=0..."
}
```

Para chamadas recebidas, a primeira negociacao WebRTC valida realiza o claim atomico para o `X-Call-Client-ID`. Tentativas de outro cliente passam a retornar `409 Conflict`.

### Aceitar chamada recebida

`POST /call/:callId/accept`

O aceite so e permitido para o proprietario depois que a ponte WebRTC estiver pronta.

### Rejeitar chamada recebida

`POST /call/:callId/reject`

Rejeita a oferta e encerra seus recursos. O endpoint existente `POST /call/reject`, com `callCreator` e `callId` no corpo, permanece disponivel e delega ao mesmo servico para evitar quebra de compatibilidade.

### Encerrar chamada

`DELETE /call/:callId`

Encerra uma chamada pertencente ao `X-Call-Client-ID`. Chamadas terminais tornam a operacao idempotente e retornam o estado final conhecido enquanto ainda estiver no periodo curto de retencao em memoria.

### Listar chamadas ativas

`GET /call/active`

Retorna apenas chamadas da instancia autenticada. O resultado pode ser filtrado pelo `X-Call-Client-ID` quando o header estiver presente.

### Eventos

`GET /call/events`

Abre um stream SSE da instancia autenticada. Quando `X-Call-Client-ID` estiver presente, o stream inclui chamadas nao assumidas e chamadas pertencentes ao cliente.

## Eventos normalizados

O envelope de evento sera:

```json
{
  "type": "call.status",
  "instanceId": "instance-id",
  "callId": "call-id",
  "clientId": "operator-id",
  "direction": "incoming",
  "status": "offered",
  "peer": "5511999999999@s.whatsapp.net",
  "timestamp": "2026-06-27T12:00:00Z",
  "reason": ""
}
```

`clientId` e `reason` sao omitidos quando nao se aplicam. Os tipos publicos iniciais sao `call.incoming`, `call.status` e `call.ended`. Os eventos nativos existentes de `CallOffer`, `CallAccept` e `CallTerminate` continuam sendo publicados para preservar compatibilidade.

## Maquina de estados

Estados suportados:

- `offered`: chamada recebida aguardando claim;
- `starting`: sinalizacao ou ponte de midia em preparacao;
- `ringing`: destino esta sendo chamado;
- `connected`: midia bidirecional ativa;
- `ending`: encerramento em andamento;
- `ended`: encerramento normal;
- `rejected`: rejeicao local ou remota;
- `failed`: falha terminal.

Fluxo de saida:

`starting -> ringing -> connected -> ending -> ended`

Fluxo de entrada:

`offered -> starting -> connected -> ending -> ended`

`rejected` e `failed` podem ser alcancados apenas a partir de estados nao terminais. Eventos repetidos do WhatsApp devem ser idempotentes.

## Concorrencia e ownership

- Cada `X-Call-Client-ID` pode possuir no maximo uma chamada nao terminal por instancia.
- Uma instancia pode manter chamadas de clientes diferentes quando o protocolo e os recursos do WhatsApp permitirem.
- A primeira negociacao WebRTC valida de uma chamada recebida realiza o claim atomico.
- Outro cliente tentando aceitar, controlar ou encerrar a mesma chamada recebe `409 Conflict`.
- A ausencia de `X-Call-Client-ID` e permitida apenas para operacoes administrativas compativeis, como a rejeicao legada; operacoes de midia exigem o header.
- A implementacao deve proteger mapas, transicoes e limpeza para passar no detector de corrida.

## Fluxos

### Chamada de saida

1. O cliente cria a chamada com `POST /call/start`.
2. O registry reserva o operador e cria o `CallManager`.
3. O cliente envia a oferta SDP e recebe a resposta SDP.
4. O manager inicia a sinalizacao pelo `WhatsAppAdapter`.
5. Eventos remotos atualizam `ringing` e `connected`.
6. O data channel transporta PCM enquanto o pipeline troca MLow/SRTP com o relay.
7. Termino local, remoto, timeout ou desconexao fecha todos os recursos e publica o estado terminal.

### Chamada de entrada

1. O handler global recebe `CallOffer` do `whatsmeow`.
2. O registry cria a chamada em `offered` e o broker publica `call.incoming`.
3. Um cliente negocia WebRTC; o registry efetua o claim atomicamente.
4. O proprietario solicita aceite e a chamada passa para `starting`.
5. O adapter aceita a chamada no WhatsApp.
6. Com transporte e midia prontos, o estado passa para `connected`.
7. O fluxo de encerramento usa a mesma limpeza da chamada de saida.

## Erros e timeouts

Erros de dominio usam os seguintes status:

- `400 Bad Request`: JSON ausente ou malformado;
- `404 Not Found`: chamada desconhecida para a instancia;
- `409 Conflict`: transicao invalida, chamada ja assumida ou operador ocupado;
- `422 Unprocessable Entity`: numero, SDP ou valor de protocolo invalido;
- `503 Service Unavailable`: instancia WhatsApp desconectada ou midia indisponivel;
- `504 Gateway Timeout`: expiracao de sinalizacao ou negociacao;
- `500 Internal Server Error`: falha inesperada.

A negociacao WebRTC tera timeout de 30 segundos. Uma oferta recebida sem claim expirara em 60 segundos, salvo termino remoto anterior. Os valores serao constantes internas inicialmente, sem nova configuracao publica.

Qualquer caminho terminal deve:

1. cancelar contextos e goroutines;
2. fechar data channel, peer connection e transporte de midia;
3. liberar ownership;
4. remover a chamada dos indices ativos;
5. publicar exatamente um evento terminal normalizado.

Uma chamada terminal pode permanecer por ate 60 segundos em um cache limitado para permitir respostas idempotentes; depois disso, consultas retornam `404`.

## Persistencia e reinicio

Chamadas e ownership ficam somente em memoria. Ao desconectar uma instancia, todos os managers associados sao encerrados. Em desligamento gracioso do processo, o registry tenta terminar as chamadas antes de fechar clientes. Em queda abrupta, a chamada remota depende do timeout do protocolo WhatsApp; nao existe recuperacao apos reinicio.

O historico sera representado por eventos e webhooks. Uma futura integracao com Chatwoot sera responsavel por persistir a chamada na conversa.

## Seguranca

- Nenhuma rota de chamada sera publica.
- O acesso inicial usa a autenticacao por API key ja existente.
- `X-Call-Client-ID` e apenas um identificador de ownership e nao concede permissao.
- SDP e eventos nao devem conter API keys nos logs.
- Frames de audio nunca serao registrados.
- O futuro widget usara tokens curtos e restritos, emitidos por backend; esse mecanismo sera especificado separadamente.

## Observabilidade

Logs estruturados devem incluir `instanceId`, `callId`, `clientId`, direcao e transicao de estado. Erros de midia registram a etapa e a causa sem incluir payload de audio. Esta entrega nao cria um novo subsistema de metricas.

## Estrategia de testes

### Unitarios

- transicoes validas e invalidas da maquina de estados;
- idempotencia de eventos repetidos;
- claim atomico e limite de uma chamada por cliente;
- expiracao e limpeza de chamadas;
- codecs e conversoes PCM/MLow;
- fechamento de todos os recursos em caminhos terminais.

### Adaptadores e HTTP

- `WhatsAppAdapter` com cliente falso;
- contratos, autenticacao, validacao e status HTTP dos handlers;
- SSE, filtros por cliente e desconexao de consumidor lento;
- compatibilidade do `POST /call/reject` existente.

### Integracao local

- negociacao WebRTC com peer local;
- PCM bidirecional atravessando data channel e pipeline falso;
- execucao com detector de corrida nos pacotes de chamada.

### Validacao real

Teste manual entre duas contas WhatsApp deve cobrir:

- chamada de saida com audio nos dois sentidos;
- chamada de entrada com aceite;
- rejeicao local e remota;
- termino local e remoto;
- desconexao da instancia durante uma chamada;
- tentativa de dois operadores aceitarem a mesma oferta.

## Criterios de aceite

- A Evolution Go realiza e recebe chamadas sem criar uma segunda sessao WhatsApp.
- Um navegador de teste negocia WebRTC e troca audio bidirecional com uma chamada real.
- Todos os endpoints retornam contratos e erros definidos nesta especificacao.
- Eventos SSE e webhooks refletem cada transicao relevante e um unico evento terminal.
- Ownership impede controle concorrente indevido.
- Desconexao, timeout e termino nao deixam chamadas, goroutines ou transportes ativos.
- O endpoint legado de rejeicao continua funcional.
- Testes automatizados dos pacotes alterados passam, incluindo verificacao de corrida onde suportada.
- Atribuicoes de licenca do WaCalls e dependencias migradas estao presentes.

## Sequencia de entrega

1. Migrar e isolar sinalizacao, transporte e codecs do WaCalls.
2. Adaptar o motor ao `whatsmeow.Client` existente.
3. Implementar registry, maquina de estados e event broker.
4. Implementar a ponte WebRTC.
5. Expor API, SSE e webhooks normalizados.
6. Adicionar testes automatizados, documentacao da API e validacao real.

O widget do Chatwoot sera especificado somente depois que essa API estiver funcional e validada.
