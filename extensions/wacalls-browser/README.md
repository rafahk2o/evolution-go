# Evolution GO WACalls Browser Extension

Extensão independente para realizar chamadas de voz WhatsApp pela Evolution GO.
Não depende de Chatwoot nem modifica páginas abertas no navegador.

## Gerar a pasta descompactada

Na raiz do repositório, execute:

```powershell
powershell -ExecutionPolicy Bypass -File extensions/wacalls-browser/scripts/package.ps1
```

O comando testa e valida a extensão, depois gera:

- `extensions/wacalls-browser/dist/`: pasta pronta para carregar descompactada;
- `extensions/wacalls-browser/artifacts/evolution-go-wacalls-browser-0.1.0.zip`: pacote opcional.

## Instalar no Chrome

1. Abra `chrome://extensions`.
2. Ative **Modo do desenvolvedor**.
3. Clique em **Carregar sem compactação**.
4. Selecione a pasta `extensions/wacalls-browser/dist`.
5. Fixe o ícone **Evolution GO WACalls** na barra do navegador.

No Microsoft Edge, use `edge://extensions`, ative o modo de desenvolvedor e
selecione **Carregar sem compactação**.

## Configurar e ligar

1. Clique no ícone da extensão para abrir a janela flutuante.
2. Informe a origem da API, por exemplo `https://evolution.exemplo.com`.
3. Informe a API key da instância e clique em **Salvar e testar**.
4. Autorize o acesso somente à origem informada.
5. Digite o telefone com DDI e DDD, por exemplo `55 11 99999-9999`.
6. Clique em **Ligar** e permita o uso do microfone.

Durante a chamada, a janela mostra status e duração e permite silenciar o
microfone ou encerrar. Fechar a janela tenta encerrar a chamada e libera os
recursos de áudio.

## Requisitos de rede

- Em produção, a API deve usar HTTPS. HTTP é aceito somente em `localhost` ou
  `127.0.0.1` para desenvolvimento.
- A instância precisa estar conectada e autenticada no WhatsApp.
- Se a Evolution GO estiver atrás de Docker ou NAT, configure `WEBRTC_PUBLIC_IP`
  e `WEBRTC_UDP_PORT` no servidor.
- Publique a mesma porta UDP no container e libere-a no firewall do servidor e
  do provedor. Sem conectividade UDP, o telefone pode tocar sem abrir o áudio.

## Segurança

A API key fica em `chrome.storage.local`, nunca em sincronização, páginas,
parâmetros de URL ou logs. A janela envia a chave ao service worker somente ao
salvar a configuração; chamadas posteriores usam mensagens sem credenciais. Um
usuário administrador do computador ainda pode inspecionar o armazenamento da
própria extensão.

## Solução de problemas

- **API key inválida:** confirme que a chave pertence à instância informada.
- **Instância desconectada:** conecte a instância no Manager e salve novamente.
- **Microfone negado:** permita o microfone nas configurações do navegador.
- **Chama, mas não há áudio:** valide `WEBRTC_PUBLIC_IP`, a publicação da porta
  UDP e as regras de firewall.
- **Mudou a URL ou chave:** abra a engrenagem e execute **Salvar e testar**.

Para atualizar, execute novamente o script de pacote e clique em **Recarregar**
na página de extensões. Para remover, use **Remover** na mesma página; isso apaga
o armazenamento local da extensão.
