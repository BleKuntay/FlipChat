# FlipChat

A real-time, one-to-one chat application featuring a social graph (friends system), built as a portfolio project.

## Tech Stack

| Layer    | Technology               |
|----------|--------------------------|
| Backend  | Go, Fiber v3, sqlx       |
| Database | PostgreSQL               |
| Cache    | Redis                    |
| Storage  | MinIO                    |
| Frontend | React (Vite, static SPA) |

## Repository Structure

```
flipchat/
├── backend/       # Go — API server
└── frontend/      # React — static SPA
```

## Status

Active Development — Phase 1 (MVP `v0.1.0`).

Features Checklist:

- [x] Auth (register, login, refresh token)
- [x] User profile
- [x] Friends & social graph
- [ ] One-to-one chat

## Local Setup

Prerequisites: Go 1.25+, Docker (for PostgreSQL & Redis)

```bash
# Clone the repository
git clone https://github.com/BleKuntay/FlipChat.git
cd FlipChat/backend

# Copy environment variables
cp .env.example .env

# Start core dependencies
docker compose up -d

# Run database migrations
make migrate-up

# Start the server
go run ./cmd/server
```

Complete documentation will be updated progressively as the project evolves.
