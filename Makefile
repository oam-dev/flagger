TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')
LT_VERSION?=$(shell grep 'VERSION' cmd/loadtester/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)

build:
	GIT_COMMIT=$$(git rev-list -1 HEAD) && CGO_ENABLED=0 GOOS=linux go build  \
		-ldflags "-s -w -X github.com/weaveworks/flagger/pkg/version.REVISION=$${GIT_COMMIT}" \
		-a -installsuffix cgo -o ./bin/flagger ./cmd/flagger/*
	docker build -t oamdev/flagger:$(TAG) . -f Dockerfile

push:
	docker tag weaveworks/flagger:$(TAG) weaveworks/flagger:$(VERSION)
	docker push weaveworks/flagger:$(VERSION)

fmt:
	gofmt -l -s -w ./
	goimports -l -w ./

test-fmt:
	gofmt -l -s ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi
	goimports -l ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi

codegen:
	./hack/update-codegen.sh

test-codegen:
	./hack/verify-codegen.sh

test: test-fmt test-codegen
	go test ./...

crd:
	cat artifacts/flagger/crd.yaml > charts/flagger/crds/crd.yaml
	cat artifacts/flagger/crd.yaml > kustomize/base/flagger/crd.yaml

run:
	go run cmd/flagger/main.go --kubeconfig ~/.kube/config --port 8088

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" pkg/version/version.go && \
	sed -i '' "s/flagger:$$current/flagger:$$next/g" artifacts/flagger/deployment.yaml && \
	sed -i '' "s/tag: $$current/tag: $$next/g" charts/flagger/values.yaml && \
	sed -i '' "s/appVersion: $$current/appVersion: $$next/g" charts/flagger/Chart.yaml && \
	sed -i '' "s/version: $$current/version: $$next/g" charts/flagger/Chart.yaml && \
	sed -i '' "s/newTag: $$current/newTag: $$next/g" kustomize/base/flagger/kustomization.yaml && \
	echo "Version $$next set in code, deployment, chart and kustomize"

release:
	git tag "v$(VERSION)"
	git push origin "v$(VERSION)"

release-notes:
	cd /tmp && GH_REL_URL="https://github.com/buchanae/github-release-notes/releases/download/0.2.0/github-release-notes-linux-amd64-0.2.0.tar.gz" && \
    curl -sSL $${GH_REL_URL} | tar xz && sudo mv github-release-notes /usr/local/bin/

loadtester-build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ./bin/loadtester ./cmd/loadtester/*
	docker build -t weaveworks/flagger-loadtester:$(LT_VERSION) . -f Dockerfile.loadtester

loadtester-push:
	docker push weaveworks/flagger-loadtester:$(LT_VERSION)

CRD_OPTIONS ?= "crd:trivialVersions=true"
# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=charts/flagger/crds

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif