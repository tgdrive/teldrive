# Integration tests

This suite runs against a real PostgreSQL instance and applies goose migrations automatically.

## Start database

```bash
docker compose -f tests/docker-compose.test.yml up -d
```

## Run tests

```bash
TEST_DATABASE_URL=postgres://teldrive_test:teldrive_test@localhost:55432/teldrive_test?sslmode=disable go test ./tests/integration/... -count=1
```

Or use the helper script:

```bash
./scripts/test-integration.sh
```

Or via task:

```bash
task test:integration
```
