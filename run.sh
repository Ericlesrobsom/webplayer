#!/bin/bash
echo "📺 IPTV WebPlayer - Setup"
echo "========================="

# Check Go
if ! command -v go &> /dev/null; then
    echo "❌ Go não encontrado. Instale Go 1.21+ em https://go.dev/dl/"
    exit 1
fi
echo "✅ Go $(go version | awk '{print $3}')"

# Check .env
if [ ! -f ".env" ]; then
    echo ""
    echo "⚠️  Arquivo .env não encontrado!"
    echo ""
    echo "📝 EDITE O .env ANTES DE CONTINUAR:"
    echo "   nano .env"
    echo ""
    echo "   Configure:"
    echo "   - SERVER_URL = URL do seu servidor IPTV"
    echo "   - SYNC_USER  = Usuário para sincronização"
    echo "   - SYNC_PASS  = Senha para sincronização"
    echo ""
    echo "   Depois rode novamente: ./run.sh"
    exit 0
fi

# Dependencies
echo "📦 Baixando dependências..."
go mod tidy

# Build
echo "🔨 Compilando (com SQLite)..."
CGO_ENABLED=1 go build -o webplayer .

if [ $? -ne 0 ]; then
    echo ""
    echo "❌ Erro na compilação."
    echo "   Se faltam dependências:"
    echo "   - Ubuntu/Debian: sudo apt install gcc sqlite3 libsqlite3-dev"
    echo "   - Alpine: apk add gcc musl-dev sqlite-dev"
    echo "   - CentOS: yum install gcc sqlite-devel"
    echo ""
    echo "   Depois rode: go mod tidy && ./run.sh"
    exit 1
fi

echo "✅ Compilado!"
echo ""
echo "📺 Iniciando servidor..."
echo "   → Acesse: http://localhost:80"
echo ""
echo "🔄 O sistema sincroniza automaticamente a cada 6h"
echo "   (configurável no .env)"
echo ""

./webplayer
