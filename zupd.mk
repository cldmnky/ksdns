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
	@podman image rm ${IMG_ZUPD} -f; \
	podman manifest exists ${IMG_ZUPD} || podman manifest create ${IMG_ZUPD}; \
	podman build --manifest ${IMG_ZUPD} --pull --platform linux/amd64,linux/arm64 -f Dockerfile.ksdns-operator bin/release && \
	podman manifest push ${IMG_ZUPD} docker://$(IMG_ZUPD)

.PHONY: sign-zupd-image
sign-zupd-image: ## Sign ksdns-operator image
	@if [ -f ksdns.key ]; then \
		${COSIGN} sign --key ksdns.key --recursive ${IMG_ZUPD}; \
	else \
		${COSIGN} sign --key env://COSIGN_PRIVATE_KEY --recursive  ${IMG_ZUPD}; \
	fi