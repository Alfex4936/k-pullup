package util

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

const (
	DELAY_THRESHOLD = 10.0
	TIME_FORMAT_STR = "2006-01-02 (Mon) 15:04:05"
)

var (
	SLACK_BOT_TOKEN              = os.Getenv("SLACK_BOT_TOKEN")
	SLACK_CHANNEL_ID             = os.Getenv("SLACK_CHANNEL_ID")
	SLACK_CHANNEL_PENDING_ID     = os.Getenv("SLACK_CHANNEL_PENDING_ID")
	SLACK_CHANNEL_NEW_MARKER_ID  = os.Getenv("SLACK_CHANNEL_NEW_MARKER_ID")
	SLACK_CHANNEL_NEW_COMMENT_ID = os.Getenv("SLACK_CHANNEL_NEW_COMMENT_ID")
	slackClient                  = slack.New(SLACK_BOT_TOKEN)
)

func SendSlackNotification(duration time.Duration, statusCode int, clientIP, method, path, userAgent, queryParams, referer string) {
	// Skip notifications for common web crawlers that may have slower response times
	if strings.Contains(userAgent, "Yeti/") || strings.Contains(userAgent, "+https://naver.me/spd") ||
		strings.Contains(userAgent, "Googlebot") || strings.Contains(userAgent, "+http://www.google.com/bot.html") {
		return
	}

	currentTime := time.Now().Format(TIME_FORMAT_STR)

	// Header section with bold text and warning emoji
	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", "*Performance Alert* (threshold: "+strconv.FormatFloat(DELAY_THRESHOLD, 'f', 2, 64)+" s) :warning:", false, false),
		nil, nil)

	// Divider to separate header from body
	dividerSection := slack.NewDividerBlock()

	// Fields for detailed information
	fields := make([]*slack.TextBlockObject, 8)
	fields[0] = slack.NewTextBlockObject("mrkdwn", "*Method:*\n`"+method+"`", false, false)
	fields[1] = slack.NewTextBlockObject("mrkdwn", "*Path:*\n`"+path+"`", false, false)
	fields[2] = slack.NewTextBlockObject("mrkdwn", "*Query:*\n`"+queryParams+"`", false, false)
	fields[3] = slack.NewTextBlockObject("mrkdwn", "*Status:*\n`"+strconv.Itoa(statusCode)+"`", false, false)
	fields[4] = slack.NewTextBlockObject("mrkdwn", "*Duration:*\n`"+duration.String()+"`", false, false)
	fields[5] = slack.NewTextBlockObject("mrkdwn", "*Client IP:*\n`"+clientIP+"`", false, false)
	fields[6] = slack.NewTextBlockObject("mrkdwn", "*User-Agent:*\n`"+userAgent+"`", false, false)
	fields[7] = slack.NewTextBlockObject("mrkdwn", "*Referer:*\n`"+referer+"`", false, false)
	bodySection := slack.NewSectionBlock(nil, fields, nil)

	// Time section
	timeSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", "*At Server Time:*\n`"+currentTime+"`", false, false),
		nil, nil)

	// Combine the message blocks
	msg := slack.MsgOptionBlocks(headerSection, dividerSection, bodySection, timeSection)

	// Post the message to Slack
	_, _, err := slackClient.PostMessage(SLACK_CHANNEL_ID, msg)
	if err != nil {
		log.Printf("Error sending message to Slack: %v\n", err)
	}
}

func SendDeploymentSuccessNotification(serverName, environment string) {
	currentTime := time.Now().Format(TIME_FORMAT_STR)

	// Header section with bold text and celebration emoji
	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", "*Deployment Success* :tada:", false, false),
		nil, nil)

	// Divider to separate header from body
	dividerSection := slack.NewDividerBlock()

	// Fields for detailed information about the deployment
	fields := make([]*slack.TextBlockObject, 3)
	fields[0] = slack.NewTextBlockObject("mrkdwn", "*Server:*\n`"+serverName+"`", false, false)
	fields[1] = slack.NewTextBlockObject("mrkdwn", "*Environment:*\n`"+environment+"`", false, false)
	fields[2] = slack.NewTextBlockObject("mrkdwn", "*Deployed At:*\n`"+currentTime+"` (UTC)", false, false)
	bodySection := slack.NewSectionBlock(nil, fields, nil)

	// Combine the message blocks
	msg := slack.MsgOptionBlocks(headerSection, dividerSection, bodySection)

	// Post the message to Slack
	_, _, err := slackClient.PostMessage(SLACK_CHANNEL_ID, msg)
	if err != nil {
		log.Printf("Error sending deployment success message to Slack: %v\n", err)
	}
}

func SendSlackReportNotification(reportDetails string) {
	currentTime := time.Now().Format(TIME_FORMAT_STR)

	// Header section with bold text and information emoji
	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", "(`"+currentTime+"`) *Daily Pending Reports* :information_source:", false, false),
		nil, nil)

	// Divider to separate header from body
	dividerSection := slack.NewDividerBlock()

	// Fields for detailed information
	bodySection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", reportDetails, false, false),
		nil, nil)

	// Combine the message blocks
	msg := slack.MsgOptionBlocks(headerSection, dividerSection, bodySection)

	// Post the message to Slack
	_, _, err := slackClient.PostMessage(SLACK_CHANNEL_PENDING_ID, msg)
	if err != nil {
		log.Printf("Error sending message to Slack: %v\n", err)
	}
}

func SendSlackNewMarkerNotification(markerID int64, address, description string, latitude, longitude float64, pics int) {
	currentTime := time.Now().Add(time.Duration(time.Hour * 9)).Format("2006-01-02 15:04:05") // show as KST

	// 마커 상세 페이지 링크
	markerURL := fmt.Sprintf("https://k-pullup.com/pullup/%d", markerID)
	markerLink := fmt.Sprintf("<%s|%d 상세 보기>", markerURL, markerID)

	// Header section with bold text
	headerText := "새로운 철봉이 등록되었습니다! :tada:"
	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", "*"+headerText+"*", false, false),
		nil, nil)

	// Divider
	dividerSection := slack.NewDividerBlock()
	naverUrl := "https://map.naver.com/?query=" + url.QueryEscape(address) + "&type=SITE_1&queryRank=0"
	naverLink := fmt.Sprintf("<%s|%s>", naverUrl, address)
	// https://map.naver.com/?query=\{encodedAddress}&type=SITE_1&queryRank=0

	// Conditionally add "(사진: pics)" if pics > 0
	picsText := ""
	if pics > 0 {
		picsText = fmt.Sprintf(" (사진: %d)", pics)
	}

	// Body section with details
	bodyText := fmt.Sprintf(
		"*주소:* %s\n*설명:* %s\n*위치:* %.6f, %.6f\n*링크:* %s%s\n*등록 시각:* %s",
		naverLink, description, latitude, longitude, markerLink, picsText, currentTime)
	bodySection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", bodyText, false, false),
		nil, nil,
	)

	msg := slack.MsgOptionBlocks(headerSection, dividerSection, bodySection)
	_, _, err := slackClient.PostMessage(SLACK_CHANNEL_NEW_MARKER_ID, msg)
	if err != nil {
		log.Printf("Error sending message to Slack: %v\n", err)
	}
}

func SendSlackNewComment(markerID int, userID int, userName, commentText string) {
	// 현재 시간 (KST)
	currentTime := time.Now().Add(time.Hour * 9).Format("2006-01-02 15:04:05")

	// 마커 상세 페이지 링크
	markerURL := fmt.Sprintf("https://k-pullup.com/pullup/%d", markerID)
	markerLink := fmt.Sprintf("<%s|마커 댓글 보기>", markerURL)

	// 헤더 섹션: 새로운 댓글 알림
	headerText := ":speech_balloon: *새로운 댓글이 등록되었습니다!*"
	headerSection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", headerText, false, false),
		nil, nil)

	// 구분선
	dividerSection := slack.NewDividerBlock()

	// 본문 섹션: 상세 정보
	bodyText := fmt.Sprintf(
		"*마커 ID:* %d\n*작성자:* %s\n*등록 시각:* %s\n*댓글 내용:*\n> %s\n*링크:* %s",
		markerID, userName, currentTime, commentText, markerLink)
	bodySection := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", bodyText, false, false),
		nil, nil,
	)

	// 메시지 조합
	msg := slack.MsgOptionBlocks(headerSection, dividerSection, bodySection)

	// 슬랙 채널로 메시지 전송
	_, _, err := slackClient.PostMessage(SLACK_CHANNEL_NEW_COMMENT_ID, msg)
	if err != nil {
		log.Printf("Error sending message to Slack: %v\n", err)
	}
}
