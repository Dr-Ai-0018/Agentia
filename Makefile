.PHONY: run-broker run-admin

run-broker:
	go run ./cmd/arena-broker

run-admin:
	go run ./cmd/arena-admin
