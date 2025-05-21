FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.18 AS build
RUN apk add upx
WORKDIR $GOPATH/src/kproximate
COPY . .
ARG COMPONENT
ARG TARGETARCH
RUN cd kproximate/$COMPONENT && GOOS=linux GOARCH=$TARGETARCH go build -v -o /go/bin/$COMPONENT
RUN upx /go/bin/$COMPONENT

FROM --platform=$BUILDPLATFORM alpine
ARG COMPONENT
RUN adduser \    
    --disabled-password \    
    --gecos "" \    
    --home "/nonexistent" \    
    --shell "/sbin/nologin" \    
    --no-create-home \    
    --uid "1001" \    
    "kproximate"

COPY --from=build /go/bin/$COMPONENT /usr/local/bin/
RUN echo -e "#!/usr/bin/env sh\n\nexec ${COMPONENT}" >> entrypoint.sh
RUN chmod +x entrypoint.sh
USER kproximate:kproximate
ENTRYPOINT ["./entrypoint.sh"]