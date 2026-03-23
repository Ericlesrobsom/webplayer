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

## COMANDOS INSTALAÇÃO

```bash
sudo apt update && sudo apt install -y golang-go gcc  unzip
```
```bash
wget https://go.dev/dl/go1.22.1.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz
```
```bash
export PATH=$PATH:/usr/local/go/bin
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.profile
```
```bash
git clone https://github.com/Ericlesrobsom/webplayer.git
```
```bash
cd webplayer
```
```bash
chmod +x run.sh && ./run.sh
```
## Configuração LIGAR ALTOMATICO

```bash
sudo nano /etc/systemd/system/webplayer.service
```
```bash
[Unit]
Description=IPTV WebPlayer
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/webplayer
ExecStart=/root/webplayer/webplayer
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```
```bash
Ctrl + c
```
# Compila primeiro

```bash
cd /root/webplayer
CGO_ENABLED=1 go build -o webplayer .
```
# Ativa o serviço

```bash
sudo systemctl daemon-reload
sudo systemctl enable webplayer
sudo systemctl start webplayer
```
# Ver status

```bash
sudo systemctl status webplayer
```
# Ver logs

```bash
sudo journalctl -u webplayer -f
```
# COMANDOS ULTILISAVEIS
```bash
sudo systemctl restart webplayer   # reiniciar
sudo systemctl stop webplayer      # parar
sudo systemctl status webplayer    # ver status
```

## Configuração (.env)

```env
========================================
  CONFIGURAÇÃO DO WEBPLAYER IPTV
========================================

# URL do servidor IPTV principal (sem barra no final)
SERVER_URL = http://king.of7seas.uk

# Porta do WebPlayer
PORT = 80

# Nome do player (aparece se não tiver logo)
PLAYER_NAME = IPTV Player

# Logo: coloque o arquivo em static/img/logo.png
# Tamanho recomendado: 200x60px, fundo transparente (PNG)
# LOGO = static/img/logo.png

# Usuário do sistema para sincronização automática
# Este usuário será usado para buscar categorias, filmes, séries a cada 6h
# IMPORTANTE: use um usuário válido do seu servidor IPTV
SYNC_USER = 693076326
SYNC_PASS = 105787916

# Intervalo de sincronização em horas (padrão: 6)
SYNC_INTERVAL = 6

# Itens por página (padrão: 100)
ITEMS_PER_PAGE = 100

# Tema/Cor personalizada (padrão: roxo #6c5ce7)
# Formato: COR_PRINCIPAL,COR_SECUNDARIA
# Exemplos:
   ACCENT = #d5c315,#776c09   (dourado)
#   ACCENT = #e74c3c,#c0392b   (vermelho)
#   ACCENT = #00b894,#00a383   (verde)
#   ACCENT = #0984e3,#74b9ff   (azul)
#   ACCENT = #e84393,#fd79a8   (rosa)
# ACCENT = #6c5ce7,#a29bfe
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
