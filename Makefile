.PHONY: test build e2e e2e-hosted e2e-envoy-gateway

test:
	go test ./... -count=1

build:
	docker build --target runtime -t nextmoca/envoy-extproc:local .

e2e:
	./scripts/e2e.sh

e2e-hosted:
	./scripts/e2e-hosted.sh

e2e-envoy-gateway:
	./scripts/e2e-envoy-gateway.sh
