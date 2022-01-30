FROM alpine:latest
COPY ./bin/factorigo-chat-bot /factorigo-chat-bot

ENTRYPOINT ["factorigo-chat-bot"]
