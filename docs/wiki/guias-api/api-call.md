# API de chamadas de voz

A Evolution Go realiza e recebe chamadas de voz usando o mesmo `whatsmeow.Client`
da instância autenticada. O áudio do navegador usa um data channel WebRTC chamado
`pcm`, com amostras signed 16-bit little-endian, mono, 16 kHz.

Todas as rotas exigem a API key normal da instância. O header
`X-Call-Client-ID` identifica o operador para ownership; ele não substitui a
autenticação.

## Iniciar chamada

`POST /call/start`

```http
X-Call-Client-ID: operador-123
Content-Type: application/json
```

```json
{"number":"5511999999999"}
```

Resposta `201 Created`:

```json
{"callId":"call-id","direction":"outgoing","status":"starting"}
```

## Negociar WebRTC

`POST /call/:callId/webrtc`

```json
{"sdpOffer":"v=0..."}
```

Resposta `200 OK`:

```json
{"sdpAnswer":"v=0..."}
```

O navegador deve criar o data channel `pcm` antes de gerar a oferta. Em chamadas
recebidas, a primeira oferta SDP válida faz o claim atômico da chamada. Outro
`X-Call-Client-ID` recebe `409 Conflict`. A negociação expira após 30 segundos.

## Aceitar chamada recebida

`POST /call/:callId/accept`

O mesmo `X-Call-Client-ID` que negociou WebRTC deve fazer o aceite. O data channel
precisa estar aberto; caso contrário, a resposta é `409 Conflict`.

## Rejeitar chamada recebida

`POST /call/:callId/reject`

Exige `X-Call-Client-ID`, rejeita a chamada e libera mídia e ownership.

O endpoint legado permanece disponível:

`POST /call/reject`

```json
{
  "callCreator":"5511999999999@s.whatsapp.net",
  "callId":"call-id"
}
```

O endpoint legado não exige `X-Call-Client-ID` e delega ao mesmo serviço quando a
chamada está no registry.

## Encerrar chamada

`DELETE /call/:callId`

Exige o proprietário. Durante a retenção terminal de 60 segundos, chamadas já
encerradas retornam o estado final de forma idempotente.

## Listar chamadas ativas

`GET /call/active`

Sem `X-Call-Client-ID`, retorna todas as chamadas ativas da instância. Com o
header, retorna chamadas do cliente e ofertas ainda não assumidas.

```json
{
  "calls":[
    {
      "instanceId":"instance-id",
      "callId":"call-id",
      "clientId":"operador-123",
      "direction":"incoming",
      "status":"connected",
      "peer":"5511999999999@s.whatsapp.net"
    }
  ]
}
```

## Eventos SSE

`GET /call/events`

O stream usa `text/event-stream`. Com `X-Call-Client-ID`, inclui ofertas não
assumidas e chamadas pertencentes ao cliente. Consumidores que esgotam o buffer
limitado são desconectados.

```text
event: call.status
data: {"type":"call.status","instanceId":"instance-id","callId":"call-id","clientId":"operador-123","direction":"incoming","status":"connected","peer":"5511999999999@s.whatsapp.net","timestamp":"2026-06-27T12:00:00Z"}
```

Os tipos normalizados são `call.incoming`, `call.status` e `call.ended`. Eles
também passam pelo mecanismo de webhook/filas quando a instância assina `CALL`.
Os eventos nativos `CallOffer`, `CallAccept` e `CallTerminate` continuam ativos.

## Estados

- `offered`: chamada recebida sem proprietário;
- `starting`: sinalização ou mídia em preparação;
- `ringing`: destino sendo chamado;
- `connected`: mídia bidirecional ativa;
- `ending`: término em andamento;
- `ended`, `rejected` e `failed`: estados terminais.

Cada cliente possui no máximo uma chamada não terminal por instância. Mapas,
claim, transições e limpeza são protegidos para execução concorrente.

## Erros

| Status | Condição |
|---|---|
| `400` | JSON inválido ou `X-Call-Client-ID` ausente |
| `404` | chamada desconhecida ou expirada |
| `409` | ownership, operador ocupado, transição inválida ou WebRTC não pronto |
| `422` | número ou SDP inválido |
| `503` | instância desconectada ou mídia indisponível |
| `504` | timeout de negociação |
| `500` | falha inesperada |

## Validação real no servidor

Use duas contas WhatsApp e valide:

1. chamada de saída com áudio nos dois sentidos;
2. chamada recebida, negociação WebRTC e aceite;
3. rejeição local e remota;
4. término local e remoto;
5. desconexão da instância durante a chamada;
6. dois operadores tentando assumir a mesma oferta.

Áudio, API keys e SDP não devem ser incluídos em logs de produção.

## Widget de teste no Manager

Na página **Instâncias**, cada instância conectada exibe um ícone de telefone na
barra de ações. O ícone abre um console para:

- iniciar uma chamada informando número com DDI e DDD;
- receber alerta por badge sem abrir o modal automaticamente;
- aceitar ou rejeitar uma chamada recebida;
- silenciar o microfone e encerrar a chamada;
- acompanhar direção, número remoto, estado e duração.

O widget gera um `X-Call-Client-ID` por navegador, usa a API key da própria
instância e consome `/call/events` em segundo plano. Para acesso ao microfone fora
de `localhost`, o Manager precisa ser servido por HTTPS. O navegador deve ter
permissão de microfone e conectividade ICE com o servidor WebRTC.
