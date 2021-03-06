TAG ?= 0.2
IMAGE_NAME = 3scaleistioadapter:$(TAG)
REGISTRY ?= quay.io/3scale
LISTEN_ADDR ?= 3333

build: ## Build the adapter binary
	dep ensure
	go build -o _output/3scaleAdapter cmd/main.go

run-adapter: ## Run the adapter
	THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} go run cmd/main.go

run-mixer-server: ## Run the mixer server with test configuration
	mixs server --configStoreURL=fs://$(CURDIR)/testdata

unit: ## Run unit tests
	mkdir -p "$(CURDIR)/_output"
	go test ./... -covermode=count -test.coverprofile="$(CURDIR)/_output/unit.cov"

integration: ## Run integration tests
	go test ./... -covermode=count -tags integration -test.coverprofile="$(CURDIR)/_output/integration.cov"

test: unit integration ## Runs all tests

unit_coverage: unit ## Runs unit tests and generates a html coverage report
	go tool cover -html="$(CURDIR)/_output/unit.cov"

integration_coverage: integration ## Runs integration tests and generates a html coverage report
	go tool cover -html="$(CURDIR)/_output/integration.cov"

debug-image: ## Builds a debuggable image which is started via Delve
	docker build -f Dockerfile.dev --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-build: ## Build builder image
	docker build --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-test: ## Runs the adapter
	docker build --tag $(IMAGE_NAME)-test .
	docker run -e THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} -ti $(IMAGE_NAME)-test

docker-push: ## Push both builder and runtime image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)
