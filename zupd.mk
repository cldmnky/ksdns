.PHONY: zupd
zupd:
	go build -o bin/zupd cmd/zupd/main.go

.PHONY: docker-build-zupd
docker-build-zupd:  ## Build docker image with zupd.
	podman build -t ${IMG_ZUPD} -f Dockerfile.zupd .

.PHONY: docker-buildx-zupd
docker-buildx-zupd: ## Build and push docker image for the zupd for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile.zupd.cross > Dockerfile.zupd.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG_ZUPD} -f Dockerfile.zupd.cross .
	- docker buildx rm project-v3-builder
	rm Dockerfile.zupd.cross