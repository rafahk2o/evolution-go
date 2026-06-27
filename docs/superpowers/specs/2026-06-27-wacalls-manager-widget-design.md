# Widget WaCalls no Manager

## Objetivo

Adicionar ao card de cada instância conectada um ícone de telefone que abra um
console completo para validar chamadas WhatsApp de entrada e saída com áudio no
navegador.

O widget é uma ferramenta de teste do motor WaCalls já integrado. Ele não cria
uma segunda sessão WhatsApp, não depende do Chatwoot e não altera os contratos
HTTP existentes.

## Escopo

O widget deve:

- inserir um botão de chamada na barra de ações de cada instância conectada;
- iniciar chamadas para um número informado pelo operador;
- detectar chamadas recebidas mesmo com o modal fechado;
- mostrar um badge pulsante no botão da instância com chamada recebida;
- negociar a ponte WebRTC antes de aceitar uma chamada recebida;
- aceitar, rejeitar e encerrar chamadas;
- transportar áudio bidirecional entre microfone, alto-falante e data channel;
- exibir direção, número remoto, estado, duração e mensagem de erro;
- permitir silenciar e reativar o microfone;
- liberar mídia, stream de eventos e peer connection ao terminar.

Não fazem parte desta entrega gravação, histórico persistente, transferência,
conferência, widget do Chatwoot ou mudanças no backend WaCalls.

## Arquitetura

O Manager contém apenas artefatos compilados. O widget será implementado como
scripts independentes em `manager/dist/assets`, seguindo o padrão dos conectores
já injetados, sem modificar o bundle React minificado.

`manager/dist/index.html` carregará:

- `wacalls-widget.js`, responsável por autenticação, descoberta dos cards,
  renderização, chamadas HTTP, SSE, WebRTC e ciclo de vida;
- `wacalls-audio-worklet.js`, responsável por captura, conversão e reprodução
  contínua de PCM.

O script principal observará mudanças no DOM para reinserir os botões quando o
React reconstruir a lista de instâncias.

## Identidade e autenticação

O widget reutiliza a lista retornada por `GET /instance/all` e usa a API key da
própria instância em todas as rotas `/call`.

Um identificador estável será gerado por navegador e persistido em
`localStorage` sob `evolution-wacalls-client-id`. O valor será enviado no header
`X-Call-Client-ID` para ownership e concorrência. Ele não substitui a API key.

API keys, SDP e frames de áudio não serão escritos no DOM, console ou logs do
widget.

## Interface

O botão terá ícone de telefone e ficará na barra de ações do card, antes das
configurações. Instâncias desconectadas terão o botão desabilitado.

O modal terá três áreas:

1. cabeçalho com instância, estado e duração;
2. conteúdo com número de destino ou dados da chamada recebida;
3. controles contextuais para ligar, aceitar, rejeitar, silenciar e encerrar.

Uma chamada recebida adiciona badge pulsante ao botão da instância. O modal não
abre automaticamente. Ao clicar no botão, o operador visualiza a oferta e decide
aceitar ou rejeitar.

Fechar o modal durante uma chamada não terminal exige confirmação. Confirmar
encerra a chamada e limpa os recursos locais; cancelar mantém o modal aberto.

## Fluxo de eventos

Cada instância conectada terá um stream `GET /call/events`. Como `EventSource`
não aceita headers customizados, o widget consumirá SSE por `fetch`, enviando
`apikey` e `X-Call-Client-ID`, e fará parsing incremental dos eventos.

O stream será reconectado com atraso limitado quando a página continuar ativa.
Eventos `call.incoming`, `call.status` e `call.ended` atualizarão badge, modal e
recursos locais. `GET /call/active` reconciliará o estado após reconexão.

## Chamada de saída

1. Solicitar permissão do microfone após ação explícita do usuário.
2. Enviar `POST /call/start` com o número normalizado.
3. Criar `RTCPeerConnection` e data channel `pcm`.
4. Gerar oferta SDP e aguardar o término da coleta ICE.
5. Enviar `POST /call/:callId/webrtc` e aplicar a resposta SDP.
6. Atualizar a interface pelos eventos SSE.

## Chamada recebida

1. Receber `call.incoming` e mostrar o badge no card correto.
2. Ao aceitar, solicitar microfone e criar o data channel `pcm`.
3. Negociar `POST /call/:callId/webrtc`, realizando o claim atômico.
4. Somente depois do data channel aberto, enviar
   `POST /call/:callId/accept`.
5. Em rejeição, enviar `POST /call/:callId/reject` sem abrir mídia.

Respostas `409` informam que outro operador assumiu a chamada e removem o badge
local após reconciliação.

## Áudio

O data channel transporta PCM signed 16-bit little-endian, mono, 16 kHz.

O AudioWorklet de captura converte a taxa nativa do microfone para 16 kHz,
agrupa frames e envia `ArrayBuffer` somente quando o data channel estiver aberto
e o microfone não estiver silenciado.

O AudioWorklet de reprodução recebe PCM de 16 kHz, converte para a taxa do
`AudioContext` e usa um buffer curto para reduzir cortes. O widget deve interromper
tracks, nodes, contexto de áudio, data channel e peer connection em todo estado
terminal.

## Erros

O modal traduzirá erros HTTP em mensagens acionáveis:

- `400` para dados ou identificador ausente;
- `404` para chamada expirada;
- `409` para ownership, operador ocupado ou transição inválida;
- `422` para número ou SDP inválido;
- `503` para instância ou mídia indisponível;
- `504` para timeout WebRTC.

Falhas de permissão do microfone devem manter a chamada controlável para rejeição
ou encerramento, sem deixar recursos parcialmente abertos.

## Validação

A validação automatizada cobrirá sintaxe dos scripts, parsing SSE, normalização
de números, transições de interface e limpeza de recursos com APIs do navegador
simuladas.

A validação manual no servidor cobrirá:

- chamada de saída com áudio nos dois sentidos;
- chamada recebida, badge, aceite e áudio;
- rejeição e encerramento local ou remoto;
- silenciar e reativar microfone;
- disputa de uma chamada em dois navegadores;
- desconexão da instância e fechamento do modal durante a chamada.

## Critérios de aceite

- Cada instância conectada exibe um único ícone de chamada funcional.
- Chamadas recebidas aparecem como badge sem abrir o modal automaticamente.
- O operador realiza todo o ciclo de chamada pelo Manager.
- O áudio trafega nos dois sentidos pelo data channel `pcm`.
- Nenhuma chamada ou recurso de mídia permanece ativo após estado terminal.
- O widget não depende do Chatwoot nem modifica o bundle React minificado.
