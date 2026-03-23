# Como gerar o APK do SNUX

## Opção 1 — Mais fácil (online, grátis)

1. Acesse: https://www.pwabuilder.com
2. Cole a URL: `https://snux.kingdev.uk`
3. Clique em "Start"
4. Ele analisa o PWA e mostra o score
5. Clique em "Package for stores" → "Android"
6. Baixe o APK gerado
7. Coloque em `static/downloads/snux.apk` na VPS

## Opção 2 — WebIntoApp (mais controle)

1. Acesse: https://webintoapp.com
2. Cole a URL: `http://SEU_IP:8080` (use HTTP aqui!)
3. Configure:
   - Nome: SNUX
   - Ícone: use o icon-512.png
   - Orientação: Qualquer
   - IMPORTANTE: Ative "Allow mixed content" nas opções
4. Gere o APK e baixe
5. Coloque em `static/downloads/snux.apk`

## Opção 3 — AppsGeyser (grátis)

1. Acesse: https://appsgeyser.com
2. Escolha "Website App"
3. Cole a URL do webplayer
4. Customize nome/ícone
5. Gere e baixe o APK

## Depois de gerar o APK

Coloque na VPS:
```bash
# Copia o APK pro servidor
scp snux.apk root@SEU_IP:/root/iptv-webplayer/static/downloads/

# Pronto! Download funciona em https://snux.kingdev.uk/download.html
```

## Sobre HTTP vs HTTPS

- O site continua em HTTPS (necessário pro download e PWA)
- Os streams de vídeo vão direto HTTP pro servidor IPTV
- Em apps nativos (APK/WebView), HTTP funciona normal
- No navegador do PC, pode dar erro de "mixed content"
- Solução pro navegador: instalar como app (PWA) ou usar o APK
