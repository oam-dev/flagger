package notifier

import (
	"bytes"
	"fmt"
	"strings"
)

type DingTalk struct {
	url      string
	username string
	channel  string
}

type DingTalkText struct {
	Text    string `json:"text,omitempty"`
	UserIds string `json:"userIds, omitempty"`
}

type DingTalkLinkText struct {
	DingTalkText `json:",inline"`
	Title        string `json:"title,omitempty"`
	MessageUrl   string `json:"messageUrl,omitempty"`
	PicUrl       string `json:"picUrl,omitempty"`
}

func NewDingTalk(url, username, channel string) (Interface, error) {
	if channel != "notify-link" && channel != "notify-text" {
		return nil, fmt.Errorf("invalid channel %s", channel)
	}
	return &DingTalk{url, username, channel}, nil
}

func (dt *DingTalk) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	var url = dt.url

	arr := strings.Split(dt.channel, "-")
	channelType := arr[0]
	contentType := arr[1]
	if strings.HasSuffix(url, "/") {
		url = fmt.Sprintf("%s%s/%s", url, channelType, contentType)
	} else {
		url = fmt.Sprintf("%s/%s/%s", url, channelType, contentType)
	}
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Application: %s\r\n", workload))
	buffer.WriteString(fmt.Sprintf("Namespace: %s\r\n", namespace))
	buffer.WriteString(fmt.Sprintf("Message: %s", message))

	var payload interface{}
	if contentType == "text" {
		payload = &DingTalkText{
			Text:    buffer.String(),
			UserIds: dt.username,
		}
	} else if contentType == "link" {
		payload = &DingTalkLinkText{
			DingTalkText: DingTalkText{
				Text:    message,
				UserIds: dt.username,
			},

			// TODO: set title and picture URL
		}
	}
	return postMessage(url, payload)
}
