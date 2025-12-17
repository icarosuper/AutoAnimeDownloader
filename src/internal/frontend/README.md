# Auto Anime Downloader - Frontend

Frontend SPA para o Auto Anime Downloader, construído com Svelte + Vite + Tailwind CSS.

## Desenvolvimento

```bash
npm install
npm run dev
```

O servidor de desenvolvimento estará disponível em `http://localhost:5173` (ou outra porta se 5173 estiver ocupada).

## Build

Para gerar os arquivos estáticos para produção:

```bash
npm run build
```

Os arquivos serão gerados no diretório `dist/`, que será servido pelo daemon.

## Variáveis de Ambiente

Crie um arquivo `.env` baseado em `.env.example`:

```bash
cp .env.example .env
```

Variáveis disponíveis:

- `VITE_API_BASE_URL`: URL base da API do daemon (padrão: `http://localhost:8091/api/v1`)
- `VITE_PORT`: Porta do servidor de desenvolvimento Vite (padrão: `5173`)

## Estrutura

- `src/routes/` - Páginas da aplicação
- `src/components/` - Componentes reutilizáveis
- `src/lib/api/` - Cliente HTTP para a API

