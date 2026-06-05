.PHONY: run-broker run-admin

run-broker:
	go run ./cmd/arena-broker

run-broker-demo:
	go run ./cmd/arena-broker --mode demo

run-admin:
	go run ./cmd/arena-admin
