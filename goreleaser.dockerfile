FROM --platform=${TARGETPLATFORM} busybox:latest
COPY pat /usr/bin/pat
ENTRYPOINT ["/usr/bin/pat"]
