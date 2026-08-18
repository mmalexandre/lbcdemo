# Demo application

## Includes:

- A Go API back-end
- A Postgres DB
- Gin routing
- Session management
- Connection to OpenAI
- Tracing with MLFlow and using a registered prompt
- Unit tests
- Integration tests
- Docker (local dev installation) + debugging set up
- VScode tasks to run common tasks

- A React client chat application

## Try the deployed Chat Demo

Quick start:

1. Open https://d13vttbhe09whf.cloudfront.net/
2. Log in with `demo` / `demo`.
3. Enter any prompt in the chat, like you would do with ChatGPT.
4. App sends LLM reply using MLFlow template as system prompt.

Important behavior to know:

- Each chat request is independent (no conversation memory between requests).
- Responses are generated using a prompt template managed in MLflow.
- Prompt and trace data are logged in MLflow so you can inspect runs and prompt versions.

## Running

### Prerequisites

Copy `.env.example` to `.env` (or create `.env`) and fill in the required values:

```
DATABASE_URL=postgres://postgres:postgres@postgres:5432/appdb?sslmode=disable
SESSION_SECRET=your-random-secret
OPENAI_API_KEY=sk-...
```

### Start everything

```bash
docker compose up --build
```

Services:
| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://localhost:8080 |
| MLflow UI | http://localhost:5000 |
| S3 storage console | http://localhost:9001 |

### Rebuild & restart only the API

```bash
docker compose up -d --build api && docker compose logs -f api
```

## Debugging the Go API (Delve)

1. Run the **"Rebuild Go and debug (Delve)"** VS Code task (or manually):
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.debug.yml up --build --force-recreate -d api && docker compose logs -f api
   ```
2. Wait until the logs show `API server listening at: [::]:2345`.
3. In VS Code press **F5** to attach with the **"Attach to Docker (Delve)"** launch config.
4. Set breakpoints in any `api/*.go` file — execution will pause when they are hit.

To go back to the normal (non-debug) API:

```bash
docker compose up -d --build --force-recreate api
```

## Production (AWS)

The app is deployed to AWS via GitHub Actions on every push to `main`. See [doc/deployment.md](doc/deployment.md) for the full setup guide.

| URL | Description |
|-----|-------------|
| https://d13vttbhe09whf.cloudfront.net | React frontend |
| https://d13vttbhe09whf.cloudfront.net/mlflow | MLflow UI |

> **API** is proxied through CloudFront at `/api/*` (same-origin HTTPS — no mixed-content issues).

### Prompt behavior troubleshooting

If chat replies become generic (and no longer follow the registered MLflow prompt), verify the API is loading prompt config correctly:

1. Confirm API config values:
   - `MLFLOW_TRACKING_URI` should point to the MLflow service base used by the API deployment.
   - `MLFLOW_PROMPT_URI` should point to your registered prompt alias/version (for example `prompts:/cardgame/production`).
2. Check API logs for fallback warnings:
   - `prompt registry: could not load ... falling back to SYSTEM_PROMPT`
3. Validate the prompt alias in MLflow exists and has the `mlflow.prompt.text` (or `mlflow.prompt.template`) tag.

Note: the Helm API Deployment includes a ConfigMap checksum annotation, so changes to API ConfigMap values trigger a pod rollout automatically on deploy.
