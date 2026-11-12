# Pinned so builds are reproducible, Dependabot keeps it up to date
FROM alpine:3.24
# Set by Docker to the architecture the image is built for (amd64 / arm64)
ARG TARGETARCH
WORKDIR /opt/project
COPY ./bin/linux-${TARGETARCH}/factorigo-chat-bot /opt/project/factorigo-chat-bot

ENTRYPOINT ["/opt/project/factorigo-chat-bot"]
