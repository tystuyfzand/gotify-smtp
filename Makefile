BUILDDIR=./build
GOTIFY_VERSION=master
PLUGIN_NAME=smtp
PLUGIN_ENTRY=plugin.go
GO_VERSION=`cat $(BUILDDIR)/gotify-server-go-version`
DOCKER_BUILD_IMAGE=gotify/build
DOCKER_WORKDIR=/proj
DOCKER_RUN=docker run --rm -v "$$PWD/.:${DOCKER_WORKDIR}" -v "`go env GOPATH`/pkg/mod/.:/go/pkg/mod" -w ${DOCKER_WORKDIR}
DOCKER_GO_BUILD=go build -mod=readonly -a -installsuffix cgo -ldflags "$$LD_FLAGS" -buildmode=plugin

download-tools:
	go install github.com/gotify/plugin-api/cmd/gomod-cap@latest

create-build-dir:
	mkdir -p ${BUILDDIR} || true

update-go-mod: create-build-dir download-tools
	wget -O ${BUILDDIR}/gotify-server.mod https://raw.githubusercontent.com/gotify/server/${GOTIFY_VERSION}/go.mod
	gomod-cap -from ${BUILDDIR}/gotify-server.mod -to go.mod
# gomod-cap only aligns dependencies that this plugin imports directly -- for
# this plugin that is just gotify/plugin-api. Go's plugin loader however
# compares every package linked into both binaries, including transitive ones
# such as gin-contrib/sse or golang.org/x/crypto. Pin all of the server's
# requirements and let `go mod tidy` drop the ones we do not need.
	grep -oE '^[[:space:]]+[a-zA-Z0-9._/-]+ v[0-9][^[:space:]]*' ${BUILDDIR}/gotify-server.mod \
		| awk '{print $$1"@"$$2}' \
		| xargs -r -n1 go mod edit -require
# Match the server's go/toolchain directives so the module graph resolves the
# same way it does upstream.
	go mod edit -go=`awk '/^go /{print $$2; exit}' ${BUILDDIR}/gotify-server.mod`
	awk '/^toolchain /{print $$2; exit}' ${BUILDDIR}/gotify-server.mod | xargs -r -I{} go mod edit -toolchain={}
	rm ${BUILDDIR}/gotify-server.mod || true
	go mod tidy

get-gotify-server-go-version: create-build-dir
	rm ${BUILDDIR}/gotify-server-go-version || true
	wget -O ${BUILDDIR}/gotify-server-go-version https://raw.githubusercontent.com/gotify/server/${GOTIFY_VERSION}/GO_VERSION

build-linux-amd64: get-gotify-server-go-version update-go-mod
	${DOCKER_RUN} ${DOCKER_BUILD_IMAGE}:$(GO_VERSION)-linux-amd64 ${DOCKER_GO_BUILD} -o ${BUILDDIR}/${PLUGIN_NAME}-linux-amd64${FILE_SUFFIX}.so ${DOCKER_WORKDIR}

build-linux-arm-7: get-gotify-server-go-version update-go-mod
	${DOCKER_RUN} ${DOCKER_BUILD_IMAGE}:$(GO_VERSION)-linux-arm-7 ${DOCKER_GO_BUILD} -o ${BUILDDIR}/${PLUGIN_NAME}-linux-arm-7${FILE_SUFFIX}.so ${DOCKER_WORKDIR}

build-linux-arm64: get-gotify-server-go-version update-go-mod
	${DOCKER_RUN} ${DOCKER_BUILD_IMAGE}:$(GO_VERSION)-linux-arm64 ${DOCKER_GO_BUILD} -o ${BUILDDIR}/${PLUGIN_NAME}-linux-arm64${FILE_SUFFIX}.so ${DOCKER_WORKDIR}

build: build-linux-arm-7 build-linux-amd64 build-linux-arm64

.PHONY: build