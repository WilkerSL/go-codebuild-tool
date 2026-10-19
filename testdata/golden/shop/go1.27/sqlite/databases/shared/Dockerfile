FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY databases/shared/go.mod databases/shared/go.sum databases/shared/
RUN cd databases/shared && go mod download
COPY databases/shared/ databases/shared/
RUN cd databases/shared \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/migrate ./cmd/migrate
RUN mkdir -p /out/data /out/certs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /app/
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/app/migrate"]
CMD ["up"]
