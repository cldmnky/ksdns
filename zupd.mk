.PHONY: build-zupd
build-zupd:
	go build -o bin/zupd cmd/zupd/main.go

.PHONY: build-zupd-release
build-zupd-release: gox generate fmt vet ## Build zupd binary.
	cd cmd/zupd && \
		${GOX} -osarch=${RELEASE_IMAGE_PLATFORMS} -output="../../bin/release/{{.OS}}/{{.Arch}}/zupd"

.PHONY: build-zupd-image
build-zupd-image:  ## Build docker image with zupd.
	podman build -t ${IMG_ZUPD} -f Dockerfile.zupd .

.PHONY: multiarch-image-zupd
multiarch-image-zupd: build-zupd-release ## Build multiarch container image with zupd.
	@podman buildx build -t ${IMG_ZUPD}-amd64 --pull --platform linux/amd64 -f Dockerfile.zupd bin/release && \
	podman buildx build -t ${IMG_ZUPD}-arm64 --pull --platform linux/arm64 -f Dockerfile.zupd bin/release && \
	podman push ${IMG_ZUPD}-arm64 && \
	podman push ${IMG_ZUPD}-amd64 && \
	podman manifest create ${IMG_ZUPD} ${IMG_ZUPD}-arm64 ${IMG_ZUPD}-amd64 && \
	podman manifest push ${IMG_ZUPD} docker://$(IMG_ZUPD) && podman image rm ${IMG_ZUPD}