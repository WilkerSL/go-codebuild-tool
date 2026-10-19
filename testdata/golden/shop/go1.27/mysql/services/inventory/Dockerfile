FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY services/inventory/go.mod services/inventory/go.sum services/inventory/
COPY contracts/go.mod contracts/
COPY databases/shared/go.mod databases/shared/
RUN cd services/inventory && go mod download
COPY contracts/ contracts/
COPY databases/shared/ databases/shared/
COPY services/inventory/ services/inventory/
RUN cd services/inventory \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/server ./cmd/server
RUN mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /app/
COPY --from=build --chown=nonroot:nonroot /out/certs /certs
ENV GRPC_ADDR=:9000 HEALTH_ADDR=:8081
EXPOSE 9000
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
