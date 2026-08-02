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
