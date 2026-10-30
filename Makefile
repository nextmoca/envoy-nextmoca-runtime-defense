.PHONY: test build chart-lint chart-test chart-package e2e e2e-hosted e2e-envoy-gateway

test:
	go test ./... -count=1

build:
	docker build --target runtime -t nextmoca/envoy-extproc:local .

chart-lint:
	helm lint charts/nextmoca-envoy-extproc

chart-test:
	./scripts/test-chart.sh

chart-package:
	helm package charts/nextmoca-envoy-extproc --destination dist

e2e:
	./scripts/e2e.sh

e2e-hosted:
	./scripts/e2e-hosted.sh

e2e-envoy-gateway:
	./scripts/e2e-envoy-gateway.sh
