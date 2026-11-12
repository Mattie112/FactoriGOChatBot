package main

import (
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/forPelevin/gomoji"
	"github.com/forewing/csgo-rcon"
	"github.com/joho/godotenv"
	"github.com/nxadm/tail"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

var (
	// The logger always exists (so nothing can hit a nil logger), main() replaces it with a configured one
	log                = logrus.New()
	messagesToDiscord  chan string
	messagesToFactorio chan string
	discordActivities  chan discordgo.Activity
	commands           chan string
	seed               string
	// VERSION These can be injected at build time: -ldflags "-X main.VERSION=dev -X main.BUILDTIME=201610251410"
	VERSION = "Undefined"
	// BUILDTIME These can be injected at build time: -ldflags "-X main.VERSION=dev -X main.BUILDTIME=201610251410"
	BUILDTIME = "Undefined"
	config    botConfig
)

var (
	// The type of a log line is the first [TAG] found, e.g. "2022-02-01 15:31:19 [JOIN] Mattie joined the game"
	reMessageType = regexp.MustCompile(`\[(\w+)]`)
	// "[CHAT] <name>: <message>"
	reChat = regexp.MustCompile(`\[CHAT] (.*?): (.*)`)
	// "[JOIN] <name> joined the game" / "[LEAVE] <name> left the game"
	reJoinLeave = regexp.MustCompile(`\[(?:JOIN|LEAVE)] (\S+)`)
	// Entries written by the companion mod: "[TYPE:field:field:...]"
	reModEntry = regexp.MustCompile(`\[([A-Z_]+):([^\]]*)]`)
	// "Online players (3):"
	rePlayersOnline = regexp.MustCompile(`\((\d+)\)`)
	// Custom Discord emoji: <:name:id> or <a:name:id>
	reCustomEmoji = regexp.MustCompile(`<a?:(\w+):\d+>`)

	luaStringEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)
)

func main() {
	// If we have an .env file -> load it
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			panic("Could not load .env file")
		}
	}

	log = getLoggerFromConfig(os.Getenv("LOG_LEVEL")) // I want the logger to be up before the config is loaded, so I cannot use the config struct here
	log.Infof("Starting FactoriGO Chat Bot %s - %s", VERSION, BUILDTIME)
	checkRequiredEnvVariables()
	var err error
	config, err = loadConfig()
	if err != nil {
		log.WithError(err).Fatal("Cannot load config")
	}

	messagesToDiscord = make(chan string)
	messagesToFactorio = make(chan string)
	commands = make(chan string)
	discordActivities = make(chan discordgo.Activity)

	// Open the log files first, without them the bot is useless
	logFiles := []string{config.factorioLog}
	if config.modLog != "" {
		logFiles = append(logFiles, config.modLog)
	}
	var tails []*tail.Tail
	for _, logFile := range logFiles {
		t, err := tailLogFile(logFile)
		if err != nil {
			log.WithError(err).WithField("file", logFile).Fatal("Failed to open log file")
		}
		tails = append(tails, t)
	}

	discord := setUpDiscord()
	rconClient := setUpRCON()

	// Setup file watchers
	for _, t := range tails {
		go processLogLines(t)
	}

	// Start functions that handle the dataflow
	go sendMessageToFactorio(rconClient)
	go sendMessageToDiscord(discord)
	go handleCommands(rconClient)

	// Notify Discord / Factorio the bot is online
	messagesToDiscord <- "FactoriGO Chat Bot is online!"
	messagesToFactorio <- "FactoriGO Chat Bot is online!"

	// Setup recurring tasks
	periodicTasks := schedule(60*time.Second, func() {
		updatePlayerCount(rconClient)
	})

	// Keep running until getting exit signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	<-sc

	// Cleanup: stop everything that produces messages. The data channels are deliberately not closed, a producer
	// that is still busy would panic on a closed channel, and the process is about to exit anyway.
	log.Info("Shutting down")
	close(periodicTasks)
	_ = discord.Close()
	for _, t := range tails {
		_ = t.Stop()
	}
}

func loadConfig() (botConfig, error) {
	ip, err := getEnvIp("RCON_IP")
	if err != nil {
		return botConfig{}, fmt.Errorf("RCON_IP: %w", err)
	}
	rconPort, err := getEnvInt("RCON_PORT")
	if err != nil {
		return botConfig{}, fmt.Errorf("RCON_PORT: %w", err)
	}
	if rconPort < 1 || rconPort > 65535 {
		return botConfig{}, fmt.Errorf("RCON_PORT: %d is not a valid port", rconPort)
	}
	return botConfig{
		discordToken:      os.Getenv("DISCORD_TOKEN"),
		discordChannelId:  os.Getenv("DISCORD_CHANNEL_ID"),
		rconIp:            ip,
		rconPort:          rconPort,
		rconPassword:      os.Getenv("RCON_PASSWORD"),
		factorioLog:       os.Getenv("FACTORIO_LOG"),
		modLog:            os.Getenv("MOD_LOG"),
		pollLog:           getEnvOrDefaultBool("POLL_LOG", false),
		allRocketLaunches: getEnvOrDefaultBool("ALL_ROCKET_LAUNCHES", false),
		achievementMode:   getEnvOrDefaultBool("ACHIEVEMENT_MODE", true),
		sendGPSPing:       getEnvOrDefaultBool("SEND_GPS_PING", false),
		sendJoinLeave:     getEnvOrDefaultBool("SEND_JOIN_LEAVE", true),
	}, nil
}

// Wrap untrusted text (player names etc.) in a Discord inline code block
func inlineCode(text string) string {
	return "`" + strings.ReplaceAll(text, "`", "'") + "`"
}

func newActivity(activityType discordgo.ActivityType, name string) *discordgo.Activity {
	return &discordgo.Activity{Name: name, Type: activityType}
}

// Parse a line from the Factorio log and format it in a way for Discord.
// Returns the message to post (empty = nothing to post) and optionally a new status for the bot.
// A line that cannot be parsed (unknown, truncated, corrupted) is ignored, it must never crash the bot.
func parseAndFormatMessage(message string) (string, *discordgo.Activity) {
	messageType := reMessageType.FindStringSubmatch(message)
	if messageType == nil {
		return "", nil
	}

	switch messageType[1] {
	case "FactoriGOChatBot":
		// Extracted to keep this function small
		return parseModLogEntries(message)
	case "CHAT":
		match := reChat.FindStringSubmatch(message)
		if match == nil {
			log.WithField("message", message).Debug("Could not parse chat message from Factorio, ignoring")
			return "", nil
		}
		name, messageContent := match[1], match[2]

		// Ignore GPS (= map pings)
		if strings.Contains(messageContent, "[gps=") {
			if config.sendGPSPing {
				return fmt.Sprintf(":map: | %s: %s", inlineCode(name), messageContent), nil
			}
			return "", nil
		}

		// When using achievement mode, ignore all messages from the server to prevent a message-loop
		if config.achievementMode && strings.HasPrefix(name, "<server>") {
			return "", nil
		}

		return fmt.Sprintf(":speech_left: | %s: %s", inlineCode(name), messageContent), nil
	case "JOIN", "LEAVE":
		if !config.sendJoinLeave {
			return "", nil
		}
		match := reJoinLeave.FindStringSubmatch(message)
		if match == nil {
			log.WithField("message", message).Debug("Could not parse join/leave message from Factorio, ignoring")
			return "", nil
		}
		if messageType[1] == "JOIN" {
			return fmt.Sprintf(":green_circle: | %s joined the game!", inlineCode(match[1])), nil
		}
		return fmt.Sprintf(":red_circle: | %s left the game!", inlineCode(match[1])), nil
	default:
		log.WithField("message", message).Debug("Could not parse message from Factorio, ignoring")
		return "", nil
	}
}

// With the companion mod (or manual edit of save game) we can extract extra information!
// Format: [FactoriGOChatBot]: "<tick> [TYPE:field:field:...]"
func parseModLogEntries(message string) (string, *discordgo.Activity) {
	entry := reModEntry.FindStringSubmatch(message)
	if entry == nil {
		return "", nil
	}
	fields := strings.Split(entry[2], ":")

	switch entry[1] {
	case "RESEARCH_STARTED":
		if fields[0] == "" {
			return "", nil
		}
		return fmt.Sprintf(":microscope: | Research started: %s", inlineCode(fields[0])), nil
	case "RESEARCH_FINISHED":
		if fields[0] == "" {
			return "", nil
		}
		return fmt.Sprintf(":microscope: | Research finished: %s", inlineCode(fields[0])),
			newActivity(discordgo.ActivityTypeListening, fields[0])
	case "PLAYER_DIED":
		// [PLAYER_DIED:name]                   -> no cause
		// [PLAYER_DIED:name:cause]             -> companion mod <= 0.5.0
		// [PLAYER_DIED:name:cause:count:total] -> companion mod >= 0.6.0
		player := fields[0]
		if player == "" {
			return "", nil
		}
		activity := newActivity(discordgo.ActivityTypeWatching, player+" dying")

		if len(fields) == 1 {
			return fmt.Sprintf(":skull: | Player died: %s (unknown cause)", inlineCode(player)), activity
		}

		cause := fields[1]
		addText := ""
		switch cause {
		case "locomotive":
			addText = " (hahaha!)"
		case "cargo-wagon", "artillery-wagon", "fluid-wagon":
			addText = " (hahaha! how the hell did you do that?!?!)"
		case "":
			cause = "unknown"
		}

		text := fmt.Sprintf(":skull: | Player died: %s, cause: %s%s", inlineCode(player), inlineCode(cause), addText)
		if len(fields) >= 4 {
			text += fmt.Sprintf(" (%s times out of %s deaths)", fields[2], fields[3])
		}
		return text, activity
	case "ROCKET_LAUNCHED":
		launchAmount, err := strconv.Atoi(fields[0])
		if err != nil {
			return "", nil
		}
		activity := newActivity(discordgo.ActivityTypeWatching, "a rocket launch")

		// By default only announce the "special" launches: the first 5, every 10th up to 100 and every 100th after that
		announce := config.allRocketLaunches ||
			launchAmount <= 5 ||
			(launchAmount >= 10 && launchAmount < 100 && launchAmount%10 == 0) ||
			(launchAmount >= 100 && launchAmount%100 == 0)
		if !announce {
			return "", activity
		}
		return fmt.Sprintf(":rocket: :rocket: :rocket: A rocket has been launched! (%d times)", launchAmount), activity
	default:
		log.WithField("message", message).Debug("Could not parse message from mod, ignoring")
		return "", nil
	}
}

func sendMessageToDiscord(discord *discordgo.Session) {
	for message := range messagesToDiscord {
		// Text coming from Factorio must never be able to ping @everyone / roles / users
		_, err := discord.ChannelMessageSendComplex(config.discordChannelId, &discordgo.MessageSend{
			Content:         message,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		})
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": message}).Error("Failed to post message to Discord")
		}
	}
}

// Build the RCON command that shows a (Discord) message in Factorio.
// The message is untrusted input: it must not be able to break out of the Lua string or start a new command.
func buildFactorioCommand(message string, achievementMode bool) string {
	// Drop control characters (newlines etc.)
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, message)

	if achievementMode {
		// Plain chat message, the prefix guarantees it never starts with a "/" (= command)
		return "[color=#7289DA][Discord]" + message + "[/color]"
	}
	return "/silent-command game.print('[color=#7289DA][Discord]" + luaStringEscaper.Replace(message) + "[/color]')"
}

func sendMessageToFactorio(rconClient RconClient) {
	log.Debugf("Setting up message handler")
	for message := range messagesToFactorio {
		cmd := buildFactorioCommand(message, config.achievementMode)
		log.WithFields(logrus.Fields{"cmd": cmd}).Debug("Sending command to Factorio (through RCON)")
		_, err := rconClient.Execute(cmd)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"cmd": cmd}).Error("Unable to send message to Factorio")
		}
	}
}

func onReceiveDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by bots and webhooks (this includes the bot itself)
	if m.Author == nil || m.Author.Bot {
		return
	}

	// Only listen on our Factorio channel
	if m.ChannelID != config.discordChannelId {
		return
	}

	log.WithFields(logrus.Fields{"message": m.Content, "author": m.Author.Username}).Debug("Received message on Discord")

	// If the message is "ping" reply with "Pong!" (no need to bother Factorio with it)
	if m.Content == "ping" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": m.Content}).Error("Failed to send message to Discord")
		}
		return
	}

	// Member is not set for every message (e.g. webhooks), so fall back to the username
	nick := m.Author.Username
	if m.Member != nil && m.Member.Nick != "" {
		nick = m.Member.Nick
	}

	// Mentions arrive as <@id>, replace them with the readable @name
	content := m.ContentWithMentionsReplaced()
	for _, attachment := range m.Attachments {
		content += "\n(attachment: " + attachment.Filename + ")"
	}

	forwardDiscordMessage(nick, content)
}

// Send a Discord message away! (to Factorio, and to the command handler if it is a command)
func forwardDiscordMessage(nick string, content string) {
	// Parse message (and handle multilines)
	messages := parseDiscordMessage(content)
	log.WithFields(logrus.Fields{"messages": messages}).Debugf("Sending Discord message to output channel")
	for _, message := range messages {
		if strings.HasPrefix(message, "!") {
			commands <- message
		}
		messagesToFactorio <- fmt.Sprintf("[%s]: %s", nick, message)
	}
}

// Convert a Discord message to lines Factorio can show, empty lines are dropped
func parseDiscordMessage(message string) []string {
	if gomoji.ContainsEmoji(message) {
		res := gomoji.FindAll(message)
		for _, emoji := range res {
			message = strings.Replace(message, emoji.Character, "**"+emoji.Slug+"**", -1)
		}
	}
	message = reCustomEmoji.ReplaceAllString(message, ":$1:")

	messages := make([]string, 0, 1)
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			messages = append(messages, line)
		}
	}
	return messages
}

// Start following a log file, only lines written from now on are reported
func tailLogFile(filename string) (*tail.Tail, error) {
	return tail.TailFile(filename, tail.Config{
		Follow:    true,
		ReOpen:    true,
		MustExist: true,
		Poll:      config.pollLog,
		Location:  &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
		// Wait for the newline, otherwise a line that is still being written is delivered in (unparsable) pieces
		CompleteLines: true,
	})
}

// Read new lines from a log file and put the parsed messages on our output channels
func processLogLines(tailFile *tail.Tail) {
	for line := range tailFile.Lines {
		if line.Err != nil {
			log.WithError(line.Err).Error("Error while tailing log file")
			continue
		}
		text := strings.TrimRight(line.Text, "\r\n")
		log.WithFields(logrus.Fields{"line": text}).Debug("Read line from Factorio log")
		message, activity := parseAndFormatMessage(text)
		if activity != nil {
			updateDiscordStatus(activity.Type, activity.Name)
		}
		if message != "" {
			messagesToDiscord <- message
		}
	}
}

func updateDiscordStatus(activityType discordgo.ActivityType, name string) {
	discordActivities <- discordgo.Activity{
		Name: name,
		Type: activityType,
	}
}

func sendDiscordStatusUpdates(discord *discordgo.Session) {
	for activity := range discordActivities {
		// Set game status
		idle := 0
		err := discord.UpdateStatusComplex(discordgo.UpdateStatusData{
			IdleSince:  &idle,
			Activities: []*discordgo.Activity{&activity},
			AFK:        false,
		})
		if err != nil {
			log.WithError(err).Errorln("Failed to update Discord status")
			continue
		}
		log.Debugln("Updated status to " + activityToStatus(&activity))
	}
}

func setUpRCON() *rcon.Client {
	address := net.JoinHostPort(config.rconIp, strconv.Itoa(config.rconPort))
	rconClient := rcon.New(address, config.rconPassword, time.Second*2)
	updatePlayerCount(rconClient)
	return rconClient
}

func getSeedFromFactorio(rconClient RconClient) string {
	if seed != "" {
		return seed
	}
	msg, err := rconClient.Execute("/seed")
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("Could not get seed from Factorio")
		return "Unknown"
	}
	seed = strings.TrimSpace(msg)
	return seed
}

func getPlayersOnlineFromFactorio(rconClient RconClient) (int, error) {
	msg, err := rconClient.Execute("/players online count")
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("Could not get player count from Factorio")
		return 0, err
	}
	match := rePlayersOnline.FindStringSubmatch(msg)
	if match == nil {
		log.WithFields(logrus.Fields{"response": msg}).Error("Could not parse player count from Factorio")
		return 0, errors.New("unexpected response to the player count command")
	}
	playersOnline, err := strconv.Atoi(match[1])
	if err != nil {
		log.WithFields(logrus.Fields{"err": err, "response": msg}).Error("Could not parse player count from Factorio")
		return 0, err
	}
	return playersOnline, nil
}

func updatePlayerCount(rconClient RconClient) {
	playersOnline, err := getPlayersOnlineFromFactorio(rconClient)
	if err != nil {
		// Already logged, keep the current status
		return
	}
	if playersOnline > 0 {
		updateDiscordStatus(discordgo.ActivityTypeWatching, "the factory grow")
	} else {
		updateDiscordStatus(discordgo.ActivityTypeWatching, "the world burn")
	}
}

func setUpDiscord() *discordgo.Session {
	discord, err := discordgo.New("Bot " + config.discordToken)
	if err != nil {
		log.WithError(err).Fatal("Could not register bot with Discord")
	}

	// Listen to incoming messages from Discord (the content of a message requires a separate, privileged, intent)
	discord.AddHandler(onReceiveDiscordMessage)
	discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	// Open socket to Discord
	if err := discord.Open(); err != nil {
		log.WithError(err).Fatal("Cannot open socket to Discord (is the 'Message Content Intent' enabled for your bot?)")
	}

	log.Infoln("Bot registered by Discord and is now listening for messages")
	go sendDiscordStatusUpdates(discord)
	// Set initial status
	updateDiscordStatus(discordgo.ActivityTypeWatching, "the world burn")
	return discord
}

func handleCommands(rconClient RconClient) {
	for command := range commands {
		switch command {
		case "!online":
			playersOnline, err := getPlayersOnlineFromFactorio(rconClient)
			if err == nil {
				messagesToDiscord <- strconv.Itoa(playersOnline) + " players online"
			}
		case "!seed":
			messagesToDiscord <- getSeedFromFactorio(rconClient)
		case "!evolution":
			msg, err := rconClient.Execute("/evolution")
			if err != nil {
				log.WithFields(logrus.Fields{"err": err}).Error("Could not get evolution from Factorio")
				msg = "Unknown"
			}
			messagesToDiscord <- msg
		}
	}
}

func checkRequiredEnvVariables() {
	vars := []string{"DISCORD_TOKEN", "DISCORD_CHANNEL_ID", "RCON_IP", "RCON_PORT", "RCON_PASSWORD", "FACTORIO_LOG"}
	for _, envVar := range vars {
		if os.Getenv(envVar) == "" {
			log.WithField("envVar", envVar).Fatal("Could not find required ENV VAR")
		}
	}
}

// See Readme for possible settings
type botConfig struct {
	discordToken      string
	discordChannelId  string
	rconIp            string
	rconPort          int
	rconPassword      string
	factorioLog       string
	modLog            string
	pollLog           bool
	allRocketLaunches bool
	achievementMode   bool
	sendGPSPing       bool
	sendJoinLeave     bool
}

type RconClient interface {
	Execute(command string) (string, error)
}
