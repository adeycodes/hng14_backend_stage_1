# Stage 1 Backend — Go + Supabase + Vercel

This repository implements the Stage 1 backend task using Go serverless functions on Vercel and Supabase (PostgreSQL) for persistence.

## Files

- `api/profiles/index.go` — `POST /api/profiles` and `GET /api/profiles`
- `api/profiles/[id].go` — `GET /api/profiles/{id}` and `DELETE /api/profiles/{id}`
- `create_table.sql` — SQL to create the `profiles` table in Supabase
- `vercel.json` — CORS response header configuration

## Setup

1. Create a Supabase project.
2. In Supabase SQL editor, run `create_table.sql`:

```sql
CREATE TABLE profiles (
    id UUID PRIMARY KEY,
    name TEXT UNIQUE,
    gender TEXT,
    gender_probability REAL,
    sample_size INTEGER,
    age INTEGER,
    age_group TEXT,
    country_id TEXT,
    country_probability REAL,
    created_at TIMESTAMP
);
```

3. In Vercel project settings, add environment variable:

- `DATABASE_URL` = `postgresql://postgres.gcxgbwtuzpqdpvssztqb:<YOUR-PASSWORD>@aws-0-eu-west-1.pooler.supabase.com:6543/postgres`

Replace `<YOUR-PASSWORD>` with your actual Supabase database password.

4. Install dependencies locally:

```bash
go mod tidy
```

5. Deploy to Vercel:

```bash
vercel
```

## Endpoints

- `POST /api/profiles`
- `GET /api/profiles`
- `GET /api/profiles/{id}`
- `DELETE /api/profiles/{id}`

## Notes

- CORS is enabled via `vercel.json` and handler headers.
- The API returns exact JSON error shapes required by the task.
- Duplicates are handled by name uniqueness in Supabase.
- `name` is stored lowercase for idempotency.

## Important

Do not commit `.env` or secret credentials.
