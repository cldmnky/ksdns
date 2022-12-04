.PHONY: zupd
zupd:
	go build -o bin/zupd cmd/zupd/main.go

.PHONY: docker-build-zupd
docker-build-zupd: test ## Build docker image with zupd.
	podman build -t ${IMG_ZUPD} -f Dockerfile.zupd .