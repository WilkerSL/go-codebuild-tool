FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY services/billing/go.mod services/billing/go.sum services/billing/
COPY contracts/go.mod contracts/
RUN cd services/billing && go mod download
COPY contracts/ contracts/
COPY services/billing/ services/billing/
RUN cd services/billing \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/migrate ./cmd/migrate
RUN mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /app/
COPY --from=build --chown=nonroot:nonroot /out/certs /certs
ENV GRPC_ADDR=:9000 HEALTH_ADDR=:8081
EXPOSE 9000
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
