# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PROJECT_NAME := cluster-manager
VERSION?=$(shell git describe --tags --dirty)
IMAGE_TAG := ${DOCKER_REGISTRY}/${PROJECT_NAME}:${VERSION}
GOSRC := github.com/kubernetes-incubator/${PROJECT_NAME}
LDFLAGS=-X main.Version=${VERSION}

all: build-image

build-image: build-local
	docker build -t $(IMAGE_TAG) -f deploy/Dockerfile . --build-arg http_proxy=$(http_proxy) --build-arg https_proxy=$(https_proxy) --build-arg no_proxy=$(no_proxy)

push-image:
	docker push ${IMAGE_TAG}

build-push-image: build-image
	docker push ${IMAGE_TAG}

build-local: test clean
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -i -ldflags "$(LDFLAGS)" -o bin/linux/cluster-manager -installsuffix cgo ./cmd/...

build-docker: test clean
	docker build -t $(IMAGE_TAG) -f deploy/Dockerfile.multistage . --build-arg LDFLAGS="$(LDFLAGS)" --build-arg http_proxy=$(http_proxy) --build-arg https_proxy=$(https_proxy) --build-arg no_proxy=$(no_proxy)

clean-image:
	docker rmi -f $(IMAGE_TAG)

gofmt:
	gofmt -w -s pkg/
	gofmt -w -s cmd/

test:
	go test ${GOSRC}/pkg/... -cover -tags test -args -v=1 -logtostderr
	go test ${GOSRC}/cmd/... -cover -tags test -args -v=1 -logtostderr

coverage: ## Generate global code coverage report
	./tools/coverage.sh;

coverhtml: ## Generate global code coverage report in HTML
	./tools/coverage.sh html;

check:
	@find . -name vendor -prune -o -name '*.go' -exec gofmt -s -d {} +
	@go vet $(shell go list ./... | grep -v '/vendor/')
	@go test -v $(shell go list ./... | grep -v '/vendor/') -tags test

vendor:
	glide install -v

clean:
	rm -rf bin
