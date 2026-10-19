FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY services/catalog/go.mod services/catalog/go.sum services/catalog/
COPY contracts/go.mod contracts/
COPY databases/shared/go.mod databases/shared/
RUN cd services/catalog && go mod download
COPY contracts/ contracts/
COPY databases/shared/ databases/shared/
COPY services/catalog/ services/catalog/
RUN cd services/catalog \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/server ./cmd/server
RUN mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /app/
COPY --from=build --chown=nonroot:nonroot /out/certs /certs
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
ENV GRPC_ADDR=:9000 HEALTH_ADDR=:8081
EXPOSE 9000
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
