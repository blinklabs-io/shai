FROM ghcr.io/blinklabs-io/go:1.22.6-1 AS build

WORKDIR /code
COPY . .
RUN make build

FROM cgr.dev/chainguard/glibc-dynamic AS shai
COPY --from=build /code/shai /bin/
ENTRYPOINT ["shai"]
