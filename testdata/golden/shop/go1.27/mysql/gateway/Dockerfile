FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY gateway/go.mod gateway/go.sum gateway/
COPY contracts/go.mod contracts/
RUN cd gateway && go mod download
COPY contracts/ contracts/
COPY gateway/ gateway/
RUN cd gateway \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/gateway ./cmd/gateway \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/devcerts ./cmd/devcerts
RUN mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /app/
COPY --from=build --chown=nonroot:nonroot /out/certs /certs
ENV HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/gateway"]
