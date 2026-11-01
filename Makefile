.PHONY: run test migrate-up docker-up docker-down
run:
	go run ./cmd/server
test:
	go test ./...
migrate-up:
	@echo "Apply migrations with your migration runner (migrations/000001_initial.up.sql)."
docker-up:
	docker compose up --build
docker-down:
	docker compose down
