FROM alpine:3.22 AS build

RUN apk add --no-cache build-base
WORKDIR /src
COPY probe.c .
RUN mkdir -p /out \
 && cc -O2 -static -fno-ident -fno-asynchronous-unwind-tables \
    -Wall -Wextra -Werror -o /out/dlbench-probe probe.c \
 && strip /out/dlbench-probe

FROM scratch
COPY --from=build /out/dlbench-probe /dlbench-probe
ENTRYPOINT ["/dlbench-probe"]
