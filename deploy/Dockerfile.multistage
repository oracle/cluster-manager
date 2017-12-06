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

FROM gcr.io/google-containers/kube-cross:v1.8.3-3

RUN useradd -u 10001 kube-operator

WORKDIR /go/src/github.com/kubernetes-incubator/cluster-manager

COPY . .

ARG LDFLAGS

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/linux/cluster-manager -installsuffix cgo -ldflags "${LDFLAGS}" ./cmd/...

RUN chmod a+x bin/linux/cluster-manager

FROM alpine:3.6
RUN apk --no-cache add ca-certificates

# FROM gcr.io/google_containers/debian-base-amd64:0.1
# RUN apt-get update && apt-get install --yes \
#   bash ca-certificates e2fsprogs systemd \
#   && apt-get clean \
#   && rm -rf /var/lib/apt/lists/*

COPY --from=0 /etc/passwd /etc/passwd

USER kube-operator

COPY --from=0 /go/src/github.com/kubernetes-incubator/cluster-manager/bin/linux/cluster-manager .

CMD ["./cluster-manager"]
