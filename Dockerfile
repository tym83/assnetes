# The image of обработчик, the runner of operators written in the 1C language.
ARG BUILDER=golang:1.26
FROM --platform=$BUILDPLATFORM ${BUILDER} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/tym83/assnetes/pkg/cli.Version=${VERSION}" \
    -o /out/obrabotchik ./cmd/obrabotchik

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/obrabotchik /obrabotchik
COPY obrabotki /obrabotki
USER 65532:65532
ENTRYPOINT ["/obrabotchik"]
