package main

import (
	"bufio"
	"flag"
	"fmt"
	"time"

	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/atomu21263/atomicgo"
	"github.com/bwmarrin/discordgo"
)

type SessionData struct {
	guildID     string
	channelID   string
	vcsession   *discordgo.VoiceConnection
	speechLimit int
	mut         sync.Mutex
	enableBot   bool
}

type UserVoiceSetting struct {
	alpha  float64 // オ－ルパス値　　   -a   0.0 - 1.0
	speed  float64 // スピーチ速度係数   -r   0.0 - inf
	pitch  float64 // 追加ハーフトーン   -fm  inf - inf
	accent float64 // F0系列内変動の重み -jf  0.0 - inf
}

//GuildIDからSessinを探す
func GetByGuildID(guildID string) (*SessionData, error) {
	for _, s := range sessions {
		if s.guildID == guildID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("cant find guild id")
}

var (
	//変数定義
	prefix            = flag.String("prefix", "", "call prefix")
	token             = flag.String("token", "", "bot token")
	openJtalkDic      = flag.String("dic", "", "OpenJtalk Dictionary")
	openJtalkVoice    = flag.String("voice", "", "OpenJtalk Dictionary")
	clientID          = ""
	sessions          = []*SessionData{}
	defaultUserConfig = UserVoiceSetting{
		alpha:  0.54,
		speed:  1,
		pitch:  0,
		accent: 3,
	}
)

func main() {
	//flag入手
	flag.Parse()
	fmt.Println("prefix       :", *prefix)
	fmt.Println("token        :", *token)

	//bot起動準備
	discord := atomicgo.DiscordBotSetup(*token)
	//eventトリガー設定
	discord.AddHandler(onReady)
	discord.AddHandler(onMessageCreate)
	discord.AddHandler(onVoiceStateUpdate)

	//起動
	atomicgo.DiscordBotStart(discord)
	defer func() {
		for _, session := range sessions {
			atomicgo.SendEmbed(discord, session.channelID, &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       "__Infomation__",
				Description: "Sorry. Bot will  Shutdown. Will be try later.",
				Color:       0xff00ff,
			})
		}
		atomicgo.DiscordBotEnd(discord)
	}() //起動メッセージ表示
	fmt.Println("Listening...")

	//bot停止対策
	atomicgo.StopWait()
}

//BOTの準備が終わったときにCall
func onReady(discord *discordgo.Session, r *discordgo.Ready) {
	clientID = discord.State.User.ID
	//1秒に1回呼び出す
	oneSecTicker := time.NewTicker(1 * time.Second)
	go func() {
		for {
			<-oneSecTicker.C
			botStateUpdate(discord)
		}
	}()
}

func botStateUpdate(discord *discordgo.Session) {
	//botのステータスアップデート
	joinedServer := len(discord.State.Guilds)
	joinedVC := len(sessions)
	VC := ""
	if joinedVC != 0 {
		VC = " " + strconv.Itoa(joinedVC) + "鯖でお話し中"
	}
	state := discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: *prefix + " help | " + strconv.Itoa(joinedServer) + "鯖で稼働中" + VC,
				Type: 0,
			},
		},
		AFK:    false,
		Status: "online",
	}
	discord.UpdateStatusComplex(state)
}

//メッセージが送られたときにCall
func onMessageCreate(discord *discordgo.Session, m *discordgo.MessageCreate) {
	mData := atomicgo.MessageViewAndEdit(discord, m)

	// 読み上げ無し のチェック
	if strings.HasPrefix(m.Content, ";") {
		return
	}

	switch {
	//TTS関連
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" join"):
		_, err := GetByGuildID(mData.GuildID)
		if err == nil {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		joinVoiceChat(mData.ChannelID, mData.GuildID, discord, mData.UserID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" get"):
		viewUserSetting(&mData, discord)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" set "):
		if strings.Count(mData.Message, " ") != 5 {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
		}
		setUserSetting(&mData, discord)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" limit "):
		session, err := GetByGuildID(mData.GuildID)
		if err != nil || session.channelID != mData.ChannelID {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		changeSpeechLimit(session, mData.Message, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" word "):
		addWord(mData.Message, mData.GuildID, discord, mData.ChannelID, mData.MessageID)
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" bot"):
		session, err := GetByGuildID(mData.GuildID)
		if err != nil {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		session.enableBot = !session.enableBot
		atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🤖")
		if session.enableBot {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🔈")
		} else {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "🔇")
		}
		return
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" leave"):
		session, err := GetByGuildID(mData.GuildID)
		if err != nil || session.channelID != mData.ChannelID {
			atomicgo.AddReaction(discord, mData.ChannelID, mData.MessageID, "❌")
			return
		}
		leaveVoiceChat(session, discord, mData.ChannelID, mData.MessageID, true)
		return
		//help
	case atomicgo.StringCheck(mData.Message, "^"+*prefix+" help"):
		sendHelp(discord, mData.ChannelID)
		return
	}

	//読み上げ
	session, err := GetByGuildID(mData.GuildID)
	if err == nil && session.channelID == mData.ChannelID && !(m.Author.Bot && !session.enableBot) {
		speechOnVoiceChat(mData.UserID, session, mData.Message)
		return
	}

}

func joinVoiceChat(channelID string, guildID string, discord *discordgo.Session, userID string, messageID string) {
	voiceConection, err := atomicgo.JoinUserVCchannel(discord, userID)
	if atomicgo.PrintError("Failed join vc", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	session := &SessionData{
		guildID:     guildID,
		channelID:   channelID,
		vcsession:   voiceConection,
		speechLimit: 50,
		mut:         sync.Mutex{},
	}
	sessions = append(sessions, session)
	atomicgo.AddReaction(discord, channelID, messageID, "✅")
	speechOnVoiceChat("BOT", session, "おはー")
}

func speechOnVoiceChat(userID string, session *SessionData, text string) {
	data, err := os.Open("./dic/" + session.guildID + ".txt")
	if atomicgo.PrintError("Failed open dictionary", err) {
		//フォルダがなかったら作成
		if !atomicgo.CheckFile("./dic") {
			if !atomicgo.CreateDir("./dic", 0755) {
				return
			}
		}
		//ふぁいる作成
		if !atomicgo.CheckFile("./dic/" + session.guildID + ".txt") {
			if !atomicgo.CreateFile("./dic/" + session.guildID + ".txt") {
				return
			}
		}
	}
	defer data.Close()

	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Text()
		replace := strings.Split(line, ",")
		text = strings.ReplaceAll(text, replace[0], replace[1])
	}

	if regexp.MustCompile(`<a:|<:|<@|<#|<@&|http|` + "```").MatchString(text) {
		text = "すーきっぷ"
	}

	//! ? { } < >を読み上げない
	replace := regexp.MustCompile(`!|\?|{|}|<|>|`)
	text = replace.ReplaceAllString(text, "")

	user, _ := userGetAndWriteConfig(userID, defaultUserConfig)

	//改行停止
	if strings.Contains(text, "\n") {
		replace := regexp.MustCompile(`\n.*`)
		text = replace.ReplaceAllString(text, "")
	}

	//隠れてるところを読み上げない
	if strings.Contains(text, "||") {
		replace := regexp.MustCompile(`\|\|.*\|\|`)
		text = replace.ReplaceAllString(text, "ピーーーー")
	}

	//text cut
	limit := session.speechLimit
	nowCount := 0
	read := ""
	for _, word := range strings.Split(text, "") {
		if nowCount < limit {
			read = read + word
			nowCount++
		}
	}

	//読み上げ待機
	session.mut.Lock()
	defer session.mut.Unlock()
	//フォルダがなかったら作成
	if !atomicgo.CheckFile("./vc") {
		if !atomicgo.CreateDir("./vc", 0755) {
			return
		}
	}
	cmd := atomicgo.ExecuteCommand("/bin/bash", "-c", "echo \""+read+"\" | open_jtalk -x "+*openJtalkDic+" -m "+*openJtalkVoice+" -a "+fmt.Sprint(user.alpha)+" -r "+fmt.Sprint(user.speed)+" -fm "+fmt.Sprint(user.pitch)+" -jf "+fmt.Sprint(user.accent)+" -ow ./vc/"+session.guildID+".wav")
	cmd.Run()
	err = atomicgo.PlayAudioFile(1, 1, session.vcsession, "./vc/"+session.guildID+".wav")
	atomicgo.PrintError("Failed play Audio ", err)
}

func viewUserSetting(m *atomicgo.MessageStruct, discord *discordgo.Session) {
	user, err := userGetAndWriteConfig(m.UserID, defaultUserConfig)
	if atomicgo.PrintError("Failed func userConfig()", err) {
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	//embedのData作成
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "",
		Description: "",
		Color:       1000,
	}

	embed.Title = "@" + m.UserData.Username + "'s Speech Config"
	embedText := "" +
		"Alpha:  " + fmt.Sprint(user.alpha) + "\n" +
		"Speed:  " + fmt.Sprint(user.speed) + "\n" +
		"Pitch:  " + fmt.Sprint(user.pitch) + "\n" +
		"Accent: " + fmt.Sprint(user.accent)
	embed.Description = embedText
	//送信
	atomicgo.SendEmbed(discord, m.ChannelID, embed)
}

//ユーザーの設定を変更
func setUserSetting(m *atomicgo.MessageStruct, discord *discordgo.Session) {
	//要らないところの切り捨て
	text := strings.Replace(m.Message, *prefix+" set ", "", 1)
	//データ編集
	user := defaultUserConfig
	fmt.Sscanf(text, "%f %f %f %f", &user.alpha, &user.speed, &user.pitch, &user.accent)
	//数値確認
	if user.alpha < 0 || 1 < user.alpha {
		atomicgo.PrintError("Alpha is not 0.0 ~ 1.0", nil)
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	if user.speed < 0.1 || 10 < user.speed {
		atomicgo.PrintError("Speed is not 0.0 ~ 10.0", nil)
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	if user.pitch < -50 || 50 < user.pitch {
		atomicgo.PrintError("Pitch is not -50.0 ~ 50.0", nil)
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	if user.accent < 0 || 50 < user.accent {
		atomicgo.PrintError("Accent is not 0.0 ~ 50.0", nil)
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	_, err := userGetAndWriteConfig(m.UserID, user)
	if atomicgo.PrintError("Failed write speed", err) {
		atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "❌")
		return
	}
	atomicgo.AddReaction(discord, m.ChannelID, m.MessageID, "🔊")
}

func userGetAndWriteConfig(userID string, data UserVoiceSetting) (user UserVoiceSetting, err error) {
	//BOTのデータを返すか
	if userID == "BOT" {
		user = UserVoiceSetting{
			alpha:  0.54,
			speed:  1,
			pitch:  0,
			accent: 3,
		}
		return
	}

	//ファイルパスの指定
	fileName := "./UserConfig.txt"

	byteText, ok := atomicgo.ReadFile(fileName)
	if !ok {
		return defaultUserConfig, fmt.Errorf("failed read&write file")
	}
	text := string(byteText)

	//書き込み用データ
	writeText := ""
	//見つかったかの判定
	pickuped := false
	//各行で検索
	for _, line := range strings.Split(text, "\n") {
		//UserIDが一緒だったら
		if strings.Contains(line, "UserID:"+userID) {
			fmt.Sscanf(line, "UserID:"+userID+" Alpha:%f Speed:%f Pitch:%f Accent:%f", &user.alpha, &user.speed, &user.pitch, &user.accent)
			pickuped = true
		} else {
			//もしないならそのまま保存
			if line != "" {
				writeText = writeText + line + "\n"
			}
		}
	}

	//0 0 0 0を返さないように対策
	if !pickuped {
		user = defaultUserConfig
	}
	//書き込みチェック用変数
	shouldWrite := false
	//上書き もしくはデータ作成
	//(userLang||userSpeed||userPitchが設定済み
	if data != defaultUserConfig {
		shouldWrite = true
	}
	if shouldWrite {
		switch {
		case data.alpha != defaultUserConfig.alpha:
			user.alpha = data.alpha
			fallthrough
		case data.speed != defaultUserConfig.speed:
			user.speed = data.speed
			fallthrough
		case data.pitch != defaultUserConfig.pitch:
			user.pitch = data.pitch
			fallthrough
		case data.accent != defaultUserConfig.accent:
			user.pitch = data.accent
		}

		//最後に書き込むテキストを追加(Write==trueの時)
		writeText = writeText + "UserID:" + userID + " Alpha:" + fmt.Sprint(user.alpha) + " Speed:" + fmt.Sprint(user.speed) + " Pitch:" + fmt.Sprint(user.pitch) + " Accent:" + fmt.Sprint(user.accent)
		//書き込み
		atomicgo.WriteFileFlash(fileName, []byte(writeText), 0777)
		log.Println("User userConfig OverWrited")
	}
	return
}

func changeSpeechLimit(session *SessionData, message string, discord *discordgo.Session, channelID string, messageID string) {
	limitText := strings.Replace(message, *prefix+" limit ", "", 1)

	limit, err := strconv.Atoi(limitText)
	if atomicgo.PrintError("Faliled limit string to int", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	if limit <= 0 || 100 < limit {
		atomicgo.PrintError("Limit is too most or too lowest.", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	session.speechLimit = limit
	atomicgo.AddReaction(discord, channelID, messageID, "🥺")
}

func addWord(message string, guildID string, discord *discordgo.Session, channelID string, messageID string) {
	text := strings.Replace(message, *prefix+" word ", "", 1)

	if !atomicgo.StringCheck(text, "^.+?,.+?$") {
		err := fmt.Errorf(text)
		atomicgo.PrintError("Check failed word", err)
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//ファイルの指定
	fileName := "./dic/" + guildID + ".txt"
	//dirがあるか確認
	if !atomicgo.CheckFile("./dic/") {
		if !atomicgo.CreateDir("./dic/", 0775) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
	}
	//fileがあるか確認
	if !atomicgo.CheckFile(fileName) {
		if !atomicgo.CreateFile(fileName) {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
			return
		}
	}
	textByte, _ := atomicgo.ReadFile(fileName)
	dic := string(textByte)

	//textをfrom toに
	from := ""
	to := ""
	_, err := fmt.Sscanf(strings.ReplaceAll(text, ",", " "), "%s %s", &from, &to)
	if atomicgo.PrintError("Failed message to dic in addWord()", err) {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	//確認
	if strings.Contains(dic, "\n"+from+",") {
		text = atomicgo.StringReplace(text, "\n", "\n"+from+",.+?\n")
	}

	dic = dic + text + "\n"
	//書き込み
	ok := atomicgo.WriteFileFlash(fileName, []byte(dic), 0777)
	if !ok {
		atomicgo.AddReaction(discord, channelID, messageID, "❌")
		return
	}

	atomicgo.AddReaction(discord, channelID, messageID, "📄")
}

func leaveVoiceChat(session *SessionData, discord *discordgo.Session, channelID string, messageID string, reaction bool) {
	speechOnVoiceChat("BOT", session, "さいなら")

	if err := session.vcsession.Disconnect(); err != nil {
		atomicgo.PrintError("Try disconect is Failed", err)
		if reaction {
			atomicgo.AddReaction(discord, channelID, messageID, "❌")
		}
		return
	} else {
		var ret []*SessionData
		for _, v := range sessions {
			if v.guildID == session.guildID {
				continue
			}
			ret = append(ret, v)
		}
		sessions = ret
		if reaction {
			atomicgo.AddReaction(discord, channelID, messageID, "⛔")
		}
		return
	}
}

func sendHelp(discord *discordgo.Session, channelID string) {
	//embedのData作成
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "BOT HELP",
		Description: "",
		Color:       1000,
	}
	Text := "--TTS--\n" +
		*prefix + " join :VCに参加します\n" +
		*prefix + " get :読み上げ設定を表示します(User単位)\n" +
		*prefix + " set <Alpha 0-1> <Speed 0.1-10> <Pitch -50-50> <Accent 0-50>: 読み上げ設定を変更します(User単位)\n" +
		*prefix + " word <元>,<先> : 辞書を登録します(Guild単位)\n" +
		*prefix + " limit <1-100> : 読み上げ文字数の上限を設定します(Guild単位)\n" +
		*prefix + " bot : Botのメッセージを読み上げるかをトグルします(Guild単位)\n" +
		*prefix + " leave : VCから切断します\n"
	embed.Description = Text
	//送信
	if _, err := discord.ChannelMessageSendEmbed(channelID, embed); err != nil {
		atomicgo.PrintError("Failed send help Embed", err)
		log.Println(err)
	}
}

//VCでJoin||Leaveが起きたときにCall
func onVoiceStateUpdate(discord *discordgo.Session, v *discordgo.VoiceStateUpdate) {

	//セッションがあるか確認
	session, err := GetByGuildID(v.GuildID)
	if err != nil {
		return
	}

	//VCに接続があるか確認
	if session.vcsession == nil || !session.vcsession.Ready {
		return
	}

	// ボイスチャンネルに誰かしらいたら return
	for _, guild := range discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if session.vcsession.ChannelID == vs.ChannelID && vs.UserID != clientID {
				return
			}
		}
	}

	// ボイスチャンネルに誰もいなかったら Disconnect する
	leaveVoiceChat(session, discord, "", "", false)
}
