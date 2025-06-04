// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package views

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/remote"
)

var prettyStatuses = map[string]string{
	model.StatusOnline:  "온라인",
	model.StatusAway:    "자리 비움",
	model.StatusDnd:     "방해 금지",
	model.StatusOffline: "오프라인",
}

func RenderStatusChangeNotificationView(events []*remote.Event, status, url string) *model.SlackAttachment {
	for _, e := range events {
		if e.Start.Time().After(time.Now()) {
			return statusChangeAttachments(e, status, url)
		}
	}

	nEvents := len(events)
	if nEvents > 0 && status == model.StatusDnd {
		return statusChangeAttachments(events[nEvents-1], status, url)
	}

	return statusChangeAttachments(nil, status, url)
}

func RenderEventWillStartLine(subject, weblink string, startTime time.Time) string {
	link, _ := url.QueryUnescape(weblink)
	eventString := fmt.Sprintf("이벤트 [%s](%s)가 곧 시작됩니다.", subject, link)
	if subject == "" {
		eventString = fmt.Sprintf("[제목 없는 이벤트](%s)가 곧 시작됩니다.", link)
	}
	if startTime.Before(time.Now()) {
		eventString = fmt.Sprintf("이벤트 [%s](%s)가 진행 중입니다.", subject, link)
		if subject == "" {
			eventString = fmt.Sprintf("[제목 없는 이벤트](%s)가 진행 중입니다.", link)
		}
	}
	return eventString
}

func renderScheduleItem(event *remote.Event, status string) string {
	if event == nil {
		return fmt.Sprintf("예정된 이벤트가 없습니다.\n상태를 %s로 다시 변경할까요?", prettyStatuses[status])
	}

	resp := RenderEventWillStartLine(event.Subject, event.Weblink, event.Start.Time())

	resp += fmt.Sprintf("\n상태를 %s로 변경할까요?", prettyStatuses[status])
	return resp
}

func statusChangeAttachments(event *remote.Event, status, url string) *model.SlackAttachment {
	actionYes := &model.PostAction{
		Name: "예",
		Integration: &model.PostActionIntegration{
			URL: url,
			Context: map[string]interface{}{
				"value":            true,
				"change_to":        status,
				"pretty_change_to": prettyStatuses[status],
				"hasEvent":         false,
			},
		},
	}

	actionNo := &model.PostAction{
		Name: "아니오",
		Integration: &model.PostActionIntegration{
			URL: url,
			Context: map[string]interface{}{
				"value":    false,
				"hasEvent": false,
			},
		},
	}

	if event != nil {
		marshalledStart, _ := json.Marshal(event.Start.Time())
		actionYes.Integration.Context["hasEvent"] = true
		actionYes.Integration.Context["subject"] = event.Subject
		actionYes.Integration.Context["weblink"] = event.Weblink
		actionYes.Integration.Context["startTime"] = string(marshalledStart)

		actionNo.Integration.Context["hasEvent"] = true
		actionNo.Integration.Context["subject"] = event.Subject
		actionNo.Integration.Context["weblink"] = event.Weblink
		actionNo.Integration.Context["startTime"] = string(marshalledStart)
	}

	title := "상태 변경"
	text := renderScheduleItem(event, status)
	sa := &model.SlackAttachment{
		Title:    title,
		Text:     text,
		Actions:  []*model.PostAction{actionYes, actionNo},
		Fallback: fmt.Sprintf("%s: %s", title, text),
	}

	return sa
}
