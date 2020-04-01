package notifier

import (
	"testing"
)

func Test_PostMessage(t *testing.T) {
	payload := &DingTalkText{
		Text:    "just test",
		UserIds: "060004",
	}
	err := postMessage(defaultUrl, payload)
	if err != nil {
		t.Fatal(err)
	}
}
