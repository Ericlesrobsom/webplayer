# 📺 IPTV WebPlayer v2

WebPlayer IPTV em Go com cache SQLite, favoritos no servidor, histórico e paginação.

## O que mudou da v1

- **Config via .env** — sem painel admin no browser, tudo na VPS
- **Logo customizada** — coloque em `static/img/logo.png`
- **Cache SQLite** — sincroniza canais/filmes/séries a cada 6h (carrega instantâneo)
- **Paginação** — 100 itens por página (configurável)
- **Favoritos no servidor** — persiste entre dispositivos (SQLite)
- **Histórico** — últimos assistidos + continuar assistindo
- **Detalhes completos** — backdrop, elenco, diretor, sinopse ao abrir filme/série
- **URLs escondidas** — nenhum link de stream aparece na interface
- **Categorias com scroll** — funciona no PC (mouse wheel) e celular
- **Login com backdrop** — posters de filmes no fundo

## Deploy Rápido

```bash
# 1. Descompacte na VPS
unzip iptv-webplayer.zip && cd iptv-webplayer

# 2. Configure o .env
cp .env.example .env
nano .env

# 3. Rode
chmod +x run.sh
./run.sh
```

## Configuração (.env)

```env
# URL do servidor IPTV (sem barra no final)
SERVER_URL = http://meuserver.com:80

# Porta
PORT = 80

# Nome (aparece se não tiver logo)
PLAYER_NAME = Meu IPTV

# Usuário para sync automático (um usuário válido do servidor)
SYNC_USER = sync_user
SYNC_PASS = sync_pass

# Intervalo de sync em horas
SYNC_INTERVAL = 6

# Itens por página
ITEMS_PER_PAGE = 100
```

## Logo

Coloque sua logo em `static/img/logo.png` (ou `.jpg`, `.svg`, `.webp`).

Se quiser outro caminho, configure no .env:
```
LOGO = static/img/minha-logo.png
```

Tamanho recomendado: 200x60px, fundo transparente.

## Como funciona

```
Browser → WebPlayer (Go + SQLite, VPS B) → Servidor IPTV (VPS A)
                  ↓
         Cache local (categorias, streams)
         Favoritos + Histórico por usuário
```

1. Na primeira execução, o sistema sincroniza tudo do servidor usando `SYNC_USER`
2. Categorias e streams ficam em SQLite local (carrega rápido)
3. A cada 6h, re-sincroniza automaticamente
4. Quando o usuário abre detalhes (filme/série), busca direto no servidor
5. Favoritos e histórico ficam no banco local, vinculados ao username

## Docker

```bash
docker build -t iptv-webplayer .
docker run -d \
  --name iptv \
  -p 8080:8080 \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/webplayer.db:/app/webplayer.db \
  -v $(pwd)/static/img:/app/static/img \
  iptv-webplayer
```

## Nginx (produção)

```nginx
server {
    listen 80;
    server_name player.meudominio.com;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
    }
}
```

HTTPS com Certbot:
```bash
sudo certbot --nginx -d player.meudominio.com
```

## Estrutura

```
iptv-webplayer/
├── main.go            # Servidor Go (API, sync, DB)
├── go.mod
├── .env               # Configuração
├── .env.example
├── run.sh
├── Dockerfile
├── webplayer.db       # SQLite (gerado automaticamente)
└── static/
    ├── index.html     # Frontend SPA
    └── img/
        └── logo.png   # Sua logo (opcional)
```

## Dependências

- Go 1.21+ com CGO (para SQLite)
- GCC instalado (`apt install gcc` no Ubuntu)
- Servidor IPTV com Xtream Codes API (player_api.php)
"# webplayv1" 
