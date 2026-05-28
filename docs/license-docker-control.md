# Controle de Licenca e Bloqueio por Assinatura no Docker

Este documento descreve como implementar um controle de licenca para instalacoes Docker da API, permitindo liberar uma chave por cliente e bloquear o uso quando o plano mensal estiver expirado, cancelado ou inadimplente.

## Objetivo

Permitir que o cliente instale a API no proprio servidor via Docker, mas que a API so funcione enquanto uma licenca ativa for validada em um servidor de licencas controlado por voce.

O comportamento esperado e:

- cliente instala a imagem Docker;
- cliente informa uma `LICENSE_KEY`;
- a API ativa a instalacao no servidor de licencas;
- a API envia heartbeats periodicos;
- se a assinatura expirar ou for cancelada, o servidor de licencas responde como inativo;
- a API local bloqueia os endpoints protegidos.

## Variaveis Docker

Separar chave comercial de chave administrativa local.

```env
LICENSE_KEY=evo_live_xxxxxxxxxxxxxxxxxxxxx
GLOBAL_API_KEY=admin_local_da_api
LICENSE_SERVER_URL=https://licencas.suaempresa.com
SERVER_PORT=8080
```

Exemplo `docker-compose.yml`:

```yaml
services:
  evolution-go:
    image: suaempresa/evolution-go:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      SERVER_PORT: 8080
      LICENSE_KEY: evo_live_xxxxxxxxxxxxxxxxxxxxx
      GLOBAL_API_KEY: admin_local_da_api
      LICENSE_SERVER_URL: https://licencas.suaempresa.com
    volumes:
      - ./data:/app/data
```

`GLOBAL_API_KEY` autentica chamadas administrativas locais. `LICENSE_KEY` valida a assinatura no servidor de licencas.

## Fluxo de Ativacao

Na inicializacao, a API deve:

1. Gerar ou carregar um `instance_id` unico da instalacao.
2. Ler `LICENSE_KEY`.
3. Chamar o servidor de licencas.
4. Persistir o estado da licenca localmente.
5. Ativar ou bloquear o runtime.

Request:

```http
POST /v1/activate
Authorization: Bearer evo_live_xxxxxxxxxxxxxxxxxxxxx
Content-Type: application/json
```

Payload:

```json
{
  "instance_id": "install_abc123",
  "version": "1.0.0"
}
```

Resposta ativa:

```json
{
  "status": "active",
  "plan": "pro",
  "expires_at": "2026-06-26T00:00:00Z",
  "max_instances": 1
}
```

Resposta bloqueada:

```http
402 Payment Required
```

```json
{
  "status": "expired",
  "reason": "payment_required"
}
```

## Fluxo de Heartbeat

A API deve enviar heartbeat periodico para confirmar que a licenca continua ativa.

Request:

```http
POST /v1/heartbeat
Authorization: Bearer evo_live_xxxxxxxxxxxxxxxxxxxxx
Content-Type: application/json
```

Payload:

```json
{
  "instance_id": "install_abc123",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "telemetry_bundle": {
    "messages_sent": 120,
    "messages_recv": 98
  }
}
```

Resposta ativa:

```json
{
  "status": "active",
  "plan": "pro",
  "expires_at": "2026-06-26T00:00:00Z"
}
```

Resposta inativa:

```http
402 Payment Required
```

```json
{
  "status": "expired",
  "reason": "payment_required"
}
```

Quando o heartbeat retornar `expired`, `canceled`, `blocked`, `inactive`, `402`, `403`, `404` ou `410`, a API deve marcar o runtime como inativo e bloquear os endpoints protegidos.

## Pontos do Codigo Atual

O projeto ja possui base de runtime/licenca em:

- `pkg/core/c0.go`
- `cmd/evolution-go/main.go`

Pontos existentes:

- `InitializeRuntime(...)`: inicializa a licenca/runtime.
- `GateMiddleware(...)`: bloqueia endpoints quando o runtime nao esta ativo.
- `LicenseRoutes(...)`: expoe `/license/status`, `/license/register` e `/license/activate`.
- `StartHeartbeat(...)`: inicia o loop periodico de heartbeat.
- `_jw(...)`: chama `/v1/activate`.
- `_ln(...)`: chama `/v1/heartbeat`.

## Alteracoes Necessarias na API Docker

### 1. Criar configuracao explicita de licenca

Adicionar no config:

```go
LicenseKey       string
LicenseServerURL string
```

Ler de:

```env
LICENSE_KEY
LICENSE_SERVER_URL
```

Evitar usar `GLOBAL_API_KEY` como licenca comercial.

### 2. Inicializar runtime com `LICENSE_KEY`

Hoje o runtime recebe `cfg.GlobalApiKey`.

Trocar para receber a chave comercial:

```go
runtimeCtx := core.InitializeRuntime(tier, version, cfg.LicenseKey)
```

Manter `GLOBAL_API_KEY` apenas para autenticar endpoints administrativos.

### 3. Tornar heartbeat autoritativo

No metodo de heartbeat, quando o servidor responder que a licenca nao esta ativa, desativar o runtime:

```go
if resp.StatusCode == http.StatusPaymentRequired ||
	resp.StatusCode == http.StatusForbidden ||
	resp.StatusCode == http.StatusNotFound ||
	resp.StatusCode == http.StatusGone {
	rc.SetInactive("license inactive")
	return fmt.Errorf("license inactive")
}
```

Criar metodo publico no `RuntimeContext`:

```go
func (rc *RuntimeContext) SetInactive(reason string) {
	rc._j9.Store(false)
}
```

Se a resposta JSON vier com status inativo:

```go
if body.Status != "active" {
	rc.SetInactive(body.Status)
	return fmt.Errorf("license status: %s", body.Status)
}
```

### 4. Manter pequeno periodo de tolerancia

Para evitar bloqueio por falha temporaria de internet, separar dois casos:

- servidor respondeu `expired/canceled/blocked`: bloquear imediatamente;
- erro de rede/timeout: permitir tolerancia curta.

Exemplo:

```go
MaxOfflineGrace = 24 * time.Hour
```

Persistir `last_successful_heartbeat_at`. Se a API ficar mais que o limite sem heartbeat valido, bloquear.

### 5. Atualizar `/license/status`

Retornar status util para suporte:

```json
{
  "status": "active",
  "instance_id": "install_abc123",
  "plan": "pro",
  "expires_at": "2026-06-26T00:00:00Z",
  "last_heartbeat_at": "2026-05-26T10:15:00Z"
}
```

Nunca retornar a licenca completa. Mostrar apenas mascarada:

```json
{
  "license_key": "evo_live...abcd"
}
```

## Servidor de Licencas

Criar uma API separada, fora da imagem Docker.

Endpoints minimos:

- `POST /v1/activate`
- `POST /v1/heartbeat`
- `POST /v1/deactivate`
- webhook do gateway de pagamento

Tabelas sugeridas:

```sql
customers
---------
id
name
email
created_at

subscriptions
-------------
id
customer_id
provider
provider_subscription_id
status
current_period_end
created_at
updated_at

licenses
--------
id
customer_id
subscription_id
license_key_hash
status
plan
expires_at
max_activations
created_at
updated_at

license_activations
-------------------
id
license_id
instance_id
version
hostname
ip_address
last_seen_at
created_at
updated_at

license_heartbeats
------------------
id
license_id
instance_id
status
messages_sent
messages_recv
created_at
```

Guardar apenas hash da licenca:

```text
sha256(license_key + server_secret)
```

## Estados de Licenca

Estados recomendados:

- `active`: pode usar.
- `trialing`: pode usar ate `expires_at`.
- `past_due`: opcional, pode usar durante periodo de tolerancia.
- `expired`: bloquear.
- `canceled`: bloquear.
- `blocked`: bloquear manualmente.

Regra simples:

```text
active/trialing => libera
past_due com tolerancia => libera temporariamente
expired/canceled/blocked => bloqueia
```

## Integracao com Pagamento

Usar webhooks do provedor de pagamento para atualizar a licenca.

Eventos comuns:

- pagamento aprovado: `license.status = active`
- falha de pagamento: `license.status = past_due`
- assinatura cancelada: `license.status = canceled`
- periodo expirou sem pagamento: `license.status = expired`

Provedores possiveis:

- Stripe
- Mercado Pago
- Asaas
- Pagar.me

## Bloqueio Local

O bloqueio deve acontecer no middleware global.

O projeto ja usa:

```go
r.Use(core.GateMiddleware(runtimeCtx))
```

Quando `RuntimeContext.IsActive()` for falso, endpoints protegidos devem retornar:

```http
503 Service Unavailable
```

```json
{
  "error": "service not activated",
  "code": "LICENSE_REQUIRED",
  "message": "License required. Open the manager to activate your license."
}
```

Endpoints que devem continuar livres:

- `/server/ok`
- `/license/status`
- `/license/register`
- `/license/activate`
- `/manager/*`
- assets estaticos

## Seguranca

Recomendacoes:

- nao colocar segredo do servidor de licencas dentro da imagem Docker;
- usar HTTPS obrigatorio no servidor de licencas;
- armazenar licencas como hash no servidor;
- nunca logar `LICENSE_KEY` completa;
- limitar ativacoes por licenca;
- vincular licenca a `instance_id`;
- registrar IP, versao e ultimo heartbeat;
- permitir bloqueio manual por suporte/admin;
- diferenciar erro de rede de licenca explicitamente cancelada.

## Limitacoes

Se a imagem Docker for open source ou facilmente modificavel, um cliente tecnico pode remover o bloqueio local. Para controle comercial mais forte:

- distribuir imagem compilada;
- manter o servidor de licencas fechado;
- validar heartbeat remoto;
- mover recursos premium para servicos remotos;
- assinar builds ou usar verificacoes de integridade.

## Checklist de Implementacao

- [ ] Adicionar `LICENSE_KEY` e `LICENSE_SERVER_URL` no config.
- [ ] Parar de usar `GLOBAL_API_KEY` como licenca comercial.
- [ ] Ajustar `InitializeRuntime` para receber `LICENSE_KEY`.
- [ ] Atualizar `/v1/activate` para validar assinatura no servidor remoto.
- [ ] Atualizar `/v1/heartbeat` para bloquear quando status nao for ativo.
- [ ] Criar `RuntimeContext.SetInactive(reason string)`.
- [ ] Persistir `last_successful_heartbeat_at`.
- [ ] Implementar tolerancia para erro de rede.
- [ ] Atualizar `/license/status`.
- [ ] Criar servidor de licencas.
- [ ] Integrar servidor de licencas com gateway de pagamento.
- [ ] Testar assinatura ativa.
- [ ] Testar assinatura expirada.
- [ ] Testar assinatura cancelada.
- [ ] Testar servidor de licencas fora do ar.
- [ ] Testar reinicio do container com licenca ativa e inativa.

