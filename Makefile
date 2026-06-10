.PHONY: proto build test lint up down clean seed

SERVICES := key-gen-service url-service redirect-service api-gateway

proto:
	protoc --proto_path=. \
		--go_out=proto --go-grpc_out=proto \
		--go_opt=module=gotiny/proto --go-grpc_opt=module=gotiny/proto \
		proto/keygen/keygen.proto proto/url/url.proto proto/redirect/redirect.proto

build:
	@for svc in $(SERVICES); do \
		echo "Building $$svc..."; \
		cd $$svc && go build ./cmd && cd ..; \
	done

test:
	@for svc in $(SERVICES); do \
		echo "Testing $$svc..."; \
		cd $$svc && go test ./... && cd ..; \
	done
	cd pkg && go test ./... && cd ..

lint:
	@for svc in $(SERVICES); do \
		echo "Vetting $$svc..."; \
		cd $$svc && go vet ./... && cd ..; \
	done

up:
	docker compose up --build -d

down:
	docker compose down

clean:
	docker compose down -v
	@for svc in $(SERVICES); do \
		rm -f $$svc/$$svc; \
	done

seed:
	@echo "Seeding key pool (1000 keys)..."
	docker compose exec postgres psql -U postgres -d gotiny -c " \
		INSERT INTO keys (code) \
		SELECT substr(md5(random()::text), 1, 7) \
		FROM generate_series(1, 1000) \
		ON CONFLICT (code) DO NOTHING;"
