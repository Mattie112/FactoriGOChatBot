# FactoriGOChatBot

A Discord chatbot for Factorio written in Golang

This bot will work without mods. It supports chat messages and player join/quit. If you want to be able to see more
information on Discord you can use the [companion mod](https://mods.factorio.com/mod/FactoriGOChatBot-companion) OR if
you don't want to use mods you can manually edit your save game!

To configure this bot use environment variables either in a `.env` file of through your OS

```
LOG_LEVEL=info (optional)
DISCORD_TOKEN=xx
DISCORD_CHANNEL_ID=xx
RCON_IP=xx
RCON_PORT=xx
RCON_PASSWORD=xx
FACTORIO_LOG=C:\Users\xx\AppData\Roaming\Factorio\console.log (Unix path also supported)
MOD_LOG=C:\Users\xx\AppData\Roaming\Factorio\script-output\factorigo-chat-bot\factorigo-chat-bot.log (Optional)
```

## Installation

### Binaries

Simply download the Unix or Windows binary from this repo and run the executeable

### Docker

Get the image from https://hub.docker.com/r/mattie112/factorigo-chat-bot and run it with the variables as listed above

### Build from source
Unix:
`GOOS=linux GARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.VERSION=$(git rev-parse --short HEAD) -X main.BUILDTIME=`date -u +%Y%m%d.%H%M%S`" -o ./bin/factorigo-chat-bot`
Windows:
`GOOS=windows GARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.VERSION=$(git rev-parse --short HEAD) -X main.BUILDTIME=$(date -u +%Y%m%d.%H%M%S)" -o ./bin/factorigo-chat-bot.exe`

If you don't have a build environment:
Run: `docker-compose run --rm builder` then copy/paste the commands listed above

## Extra data supported:

- Research started / finished
- Player deaths
- Rocket launched

## Extra data without mod

- Extract your factorio save/zip file
- Add the contents of [control.lua](https://github.com/Mattie112/FactoriGOChatBot-companion/blob/main/control.lua) to
  the `control.lua` in your save
- Re-zip your save file