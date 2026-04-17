# HNG Stage 1 — Profile Intelligence Service (Vercel + Go)

Accepts a name, enriches it via Genderize, Agify, and Nationalize APIs concurrently, stores the result in PostgreSQL, and exposes 4 REST endpoints.

## Project Structure

```
├── api/
│   ├── profiles.go          → POST /api/profiles, GET /api/profiles
│   └── profiles/
│       └── [id].go          → GET /api/profiles/{id}, DELETE /api/profiles/{id}
├── internal/
│   └── shared/
│       └── shared.go        → DB, models, helpers (shared by both handlers)
├── go.mod
├── vercel.json
└── README.md
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/profiles` | Create profile (idempotent) |
| `GET` | `/api/profiles` | List profiles (filterable) |
| `GET` | `/api/profiles/{id}` | Get single profile |
| `DELETE` | `/api/profiles/{id}` | Delete profile |

## Filters (GET /api/profiles)

All filters are optional and case-insensitive:
- `?gender=male`
- `?country_id=NG`
- `?age_group=adult`
- Combine: `?gender=male&country_id=NG`

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string (from Supabase) |

## Deploy

### 1. Set up Supabase (free Postgres)
- Go to https://supabase.com → New Project
- Copy the **Connection String** from Settings → Database → Connection string (URI mode)
- It looks like: `postgres://postgres:[password]@db.[ref].supabase.co:5432/postgres`

### 2. Deploy to Vercel
```bash
npm i -g vercel
vercel login
vercel --prod
```
When prompted, add `DATABASE_URL` as an environment variable.

Or via Vercel dashboard:
- Import your GitHub repo
- Add `DATABASE_URL` in Environment Variables
- Deploy

### 3. Test
```bash
curl -X POST https://your-app.vercel.app/api/profiles \
  -H "Content-Type: application/json" \
  -d '{"name": "james"}'
```

## Running Locally

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/hng_stage1?sslmode=disable"
vercel dev
```

## Tech Stack

- **Go 1.22** — serverless functions (standard library HTTP)
- **Vercel** — serverless deployment
- **PostgreSQL** (Supabase) — data persistence
- **lib/pq** — Postgres driver
- **google/uuid** — UUID v7 generation