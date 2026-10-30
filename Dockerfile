# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/nextmoca-envoy-extproc ./cmd/nextmoca-envoy-extproc \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mock-nextmoca ./cmd/mock-nextmoca \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mock-provider ./cmd/mock-provider

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/nextmoca-envoy-extproc /nextmoca-envoy-extproc
USER nonroot:nonroot
EXPOSE 9002 8080
ENTRYPOINT ["/nextmoca-envoy-extproc"]

# This target is only for the local end-to-end test. It contains deterministic
# HTTP fixtures, not Next Moca selection or security logic.
FROM gcr.io/distroless/static-debian12:nonroot AS e2e
COPY --from=build /out/mock-nextmoca /mock-nextmoca
COPY --from=build /out/mock-provider /mock-provider
USER nonroot:nonroot
