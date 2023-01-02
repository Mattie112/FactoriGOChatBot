package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/forewing/csgo-rcon"
	"github.com/forPelevin/gomoji"
	"github.com/joho/godotenv"
	"github.com/nxadm/tail"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	log                *logrus.Logger
	messagesToDiscord  chan string
	messagesToFactorio chan string
	discordActivities  chan discordgo.Activity
	commands           chan string
	discordChannelId   string
	playersOnline      int
	seed               string
	// VERSION These can be injected at build time -ldflags "-InputArgs main.VERSION=dev main.BUILD_TIME=201610251410"
	VERSION = "Undefined"
	// BUILDTIME These can be injected at build time -ldflags "-InputArgs main.VERSION=dev main.BUILD_TIME=201610251410"
	BUILDTIME = "Undefined"
	config    sConfig
)

func main() {
	// If we have an .env file -> load it
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			panic("Could not load .env file")
		}
	}

	log = getLoggerFromConfig(os.Getenv("LOG_LEVEL"))
	log.Infof("Starting FactoriGO Chat Bot %s - %s", VERSION, BUILDTIME)
	checkRequiredEnvVariables()
	config = loadConfig() // Load optional config

	discordChannelId = os.Getenv("DISCORD_CHANNEL_ID")

	messagesToDiscord = make(chan string)
	messagesToFactorio = make(chan string)
	commands = make(chan string)

	discord := setUpDiscord()
	rconClient := setUpRCON()

	//Setup file watchers
	go readFactorioLogFile(os.Getenv("FACTORIO_LOG"))
	if os.Getenv("MOD_LOG") != "" {
		go readFactorioLogFile(os.Getenv("MOD_LOG"))
	}

	// Start functions that handle the dataflow
	go sendMessageToFactorio(rconClient)
	go sendMessageToDiscord(discord)
	go handleCommands(discord, rconClient)

	// Keep running until getting exit signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanup
	_ = discord.Close()
}

func loadConfig() sConfig {
	var c = sConfig{allRocketLaunches: getenvBool("ALL_ROCKET_LAUNCHES")}
	return c
}

// Parse the message and format it in a way for Discord
func parseAndFormatMessage(message string) string {
	var re = regexp.MustCompile(`(?m)\[(\w*)]`)
	messageType := re.FindStringSubmatch(message)

	if len(messageType) < 2 {
		return ""
	}

	switch messageType[1] {
	case "FactoriGOChatBot":
		// Extracted to keep this function small
		return parseModLogEntries(message)
	case "CHAT":
		var re = regexp.MustCompile(`(?m)] (.*): (.*)`)
		match := re.FindStringSubmatch(message)

		// Ignore GPS (= map pings)
		messageContent := match[2]
		if strings.Contains(messageContent, "[gps=") {
			return ""
		}
		return fmt.Sprintf(":speech_left: | `%s`: %s", match[1], messageContent)
	case "JOIN":
		commands <- "playerCount"
		var re = regexp.MustCompile(`(?m)] (\w*)`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":green_circle: | `%s` joined the game!", match[1])
	case "LEAVE":
		commands <- "playerCount"
		var re = regexp.MustCompile(`(?m)] (\w*)`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":red_circle: | `%s` left the game!", match[1])
	default:
		log.WithField("message", message).Debug("Could not parse message from Factorio, ignoring")
		return ""
	}
}

// With the companion mod (or manual edit of save game) we can extract extra information!
func parseModLogEntries(message string) string {
	var re = regexp.MustCompile(`(?mU) \[(.*):`)
	messageType := re.FindStringSubmatch(message)

	if len(messageType) < 1 {
		return ""
	}

	switch messageType[1] {
	case "RESEARCH_STARTED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		return fmt.Sprintf(":microscope: | Research started: `%s`", match[1])
	case "RESEARCH_FINISHED":
		var re = regexp.MustCompile(`(?m):(\S*)]`)
		match := re.FindStringSubmatch(message)
		updateDiscordStatus(discordgo.ActivityTypeListening, match[1])
		return fmt.Sprintf(":microscope: | Research finished: `%s`", match[1])
	case "PLAYER_DIED":
		var re = regexp.MustCompile(`(?m):([\w -]*)+`)
		match := re.FindAllStringSubmatch(message, -1)

		updateDiscordStatus(discordgo.ActivityTypeStreaming, match[1][1] + " dying")
		// No cause
		if len(match) == 2 {
			return fmt.Sprintf(":skull: | Player died: `%s` (unknown cause)", match[1][1])
		}

		cause := match[2][1]
		addText := ""

		if cause == "locomotive" || cause == "cargo-wagon" || cause == "artillery-wagon" || cause == "fluid-wagon" {
			addText = " (hahaha!)"
		}

		if cause == "cargo-wagon" || cause == "artillery-wagon" || cause == "fluid-wagon" {
			addText = " (hahaha! how the hell did you do that?!?!)"
		}

		if cause == "" {
			cause = "unknown"
		}

		// Only cause (companion mod <= 0.5.0
		if len(match) == 3 {
			return fmt.Sprintf(":skull: | Player died: `%s`, cause: `%s`%s", match[1][1], cause, addText)
		}

		// Cause and death count (companion mod >= 0.6.0)
		if len(match) == 5 {
			return fmt.Sprintf(":skull: | Player died: `%s`, cause: `%s`%s (%s times out of %s deaths)", match[1][1], cause, addText, match[3][1], match[4][1])
		}

		return ""
	case "ROCKET_LAUNCHED":
		updateDiscordStatus(discordgo.ActivityTypeStreaming, "a rocket launch")
		var re = regexp.MustCompile(`(?m):(\d*)]`)
		match := re.FindStringSubmatch(message)
		launchAmount, _ := strconv.Atoi(match[1])

		if config.allRocketLaunches {
			return fmt.Sprintf(":rocket: :rocket: :rocket: A rocket has been launched! (%d times)", launchAmount)
		} else {
			switch {
			case launchAmount <= 5:
				fallthrough
			case launchAmount >= 10 && launchAmount < 100 && launchAmount%10 == 0:
				fallthrough
			case launchAmount >= 100 && launchAmount%100 == 0:
				return fmt.Sprintf(":rocket: :rocket: :rocket: A rocket has been launched! (%d times)", launchAmount)
			}
		}
		return ""
	default:
		log.WithField("message", message).Debug("Could not parse message from mod, ignoring")
		return ""
	}
}

func sendMessageToDiscord(discord *discordgo.Session) {
	for message := range messagesToDiscord {
		_, err := discord.ChannelMessageSend(discordChannelId, message)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": message}).Error("Failed to post message to Discord")
		}
	}
}

func sendMessageToFactorio(rconClient *rcon.Client) {
	log.Debugf("Setting up message handler")
	for message := range messagesToFactorio {
		message = strings.Replace(message, "'", "\\'", -1)
		cmd := "/silent-command game.print('[color=#7289DA][Discord]" + message + "[/color]')"
		log.WithFields(logrus.Fields{"cmd": cmd}).Debug("Sending command to Factorio (through RCON)")
		_, err := rconClient.Execute(cmd)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"cmd": cmd}).Error("Unable to send message to Factorio")
		}
	}
}

func onReceiveDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messagesToDiscord created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Only listen on our Factorio channel
	if m.ChannelID != discordChannelId {
		return
	}

	log.WithFields(logrus.Fields{"message": m.Content, "author": m.Author}).Debug("Received message on Discord")

	// If the message is "ping" reply with "Pong!"
	if m.Content == "ping" {
		_, err := s.ChannelMessageSend(m.ChannelID, "Pong!")
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"message": m.Content}).Error("Failed to send message to Discord")
		}
	}

	// Send message away!
	nick := m.Member.Nick
	if nick == "" {
		nick = m.Author.Username
	}

	// Parse message (and handle multilines)
	messages := parseDiscordMessage(m.Content)
	log.WithFields(logrus.Fields{"messages": messages}).Debugf("Sending Discord message to output channel")
	for _, message := range messages {
		if len(message) > 0 && message[0:1] == "!" {
			commands <- message
		}
		messagesToFactorio <- fmt.Sprintf("[%s]: %s", nick, message)
	}
}

func parseDiscordMessage(message string) []string {
	if gomoji.ContainsEmoji(message) {
		res := gomoji.FindAll(message)
		for _, emoji := range res {
			message = strings.Replace(message, emoji.Character, "**"+emoji.Slug+"**", -1)
		}
	}
	messages := strings.Split(message, "\n")
	return messages
}

// Read the last line of a file and puts the parsed message on our output channel
func readFactorioLogFile(filename string) {
	t, err := tail.TailFile(filename, tail.Config{
		Follow: true,
		ReOpen: true,
		MustExist: true,
		Poll: os.Getenv("POLL_LOG") != "",
	})
	if err != nil {
		log.WithError(err).Error("Failed to open mod log file")
		return
	}
	for line := range t.Lines {
		log.WithFields(logrus.Fields{"line": line.Text}).Debug("Read line from Factorio log")
		message := parseAndFormatMessage(line.Text)
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
		var idle int
		idle = int(0)
		discord.UpdateStatusComplex(discordgo.UpdateStatusData{
			IdleSince: &idle,
			Activities: []*discordgo.Activity{ &activity, },
			AFK: false,
		})
		log.Debugln("Updated status to " + activityToStatus(&activity))
	}
}

func setUpRCON() *rcon.Client {
	rconIp := os.Getenv("RCON_IP")
	rconPort := os.Getenv("RCON_PORT")
	rconPassword := os.Getenv("RCON_PASSWORD")
	rconClient := rcon.New(rconIp+":"+rconPort, rconPassword, time.Second*2)
	updatePlayerCount(rconClient)
	msg, err := rconClient.Execute("/seed")
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("Could not get seed from Factorio")
		seed = "Unknown"
	} else {
		seed = msg
	}
	return rconClient
}

func updatePlayerCount(rconClient *rcon.Client){
	msg, err := rconClient.Execute("/players online count")
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Error("Could not get player count from Factorio")
		playersOnline = -1
		return
	}
	playersOnline, err = strconv.Atoi(strings.Split(strings.Split(msg, "(")[1], ")")[0])
	if err != nil {
		log.WithFields(logrus.Fields{"err": err}).Panic("Could not parse player count from Factorio")
		playersOnline = -1
	}
	if playersOnline > 0 {
		updateDiscordStatus(discordgo.ActivityTypeWatching, "the factory grow")
	} else {
		updateDiscordStatus(discordgo.ActivityTypeWatching, "the world burn")
	}
}

func setUpDiscord() *discordgo.Session {
	discordToken := os.Getenv("DISCORD_TOKEN")

	discord, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.WithFields(logrus.Fields{"err": err, "token": discordToken}).Panic("Could register bot with Discord")
	}

	// Listen to incoming messagesToDiscord from Discord
	discord.AddHandler(onReceiveDiscordMessage)
	discord.Identify.Intents = discordgo.IntentsGuildMessages

	// Open socket to Discord
	if discord.Open() != nil {
		log.WithFields(logrus.Fields{"err": err}).Panic("Cannot open socket to Discord")
	}

	log.Infoln("Bot registered by Discord and is now listening for messagesToDiscord")
	discordActivities = make(chan discordgo.Activity)
	go sendDiscordStatusUpdates(discord)
	// Set initial status
	updateDiscordStatus(discordgo.ActivityTypeWatching, "the world burn")
	return discord
}

func handleCommands(discord *discordgo.Session, rconClient *rcon.Client){
	for command := range commands {
		switch(command){
			case "!online":
				discord.ChannelMessageSend(discordChannelId, strconv.Itoa(playersOnline) + " players online")
				break
			case "!seed":
				discord.ChannelMessageSend(discordChannelId, seed)
				break
			case "!evolution":
				msg, err := rconClient.Execute("/evolution")
				if err != nil {
					log.WithFields(logrus.Fields{"err": err}).Error("Could not get evolution from Factorio")
					msg = "Unknown"
				}
				discord.ChannelMessageSend(discordChannelId, msg)
				break
			case "playerCount":
				// This is only triggered in code, never by a message
				updatePlayerCount(rconClient)
				break
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


type sConfig struct {
	allRocketLaunches bool
}
