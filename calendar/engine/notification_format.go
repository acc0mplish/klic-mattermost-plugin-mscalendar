// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/engine/views"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/remote"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/fields"

	"github.com/mattermost/mattermost/server/public/model"
)

func (processor *notificationProcessor) newSlackAttachment(n *remote.Notification) *model.SlackAttachment {
	title := views.EnsureSubject(n.Event.Subject)
	titleLink := n.Event.Weblink
	text := n.Event.BodyPreview
	return &model.SlackAttachment{
		AuthorName: n.Event.Organizer.EmailAddress.Name,
		AuthorLink: "mailto:" + n.Event.Organizer.EmailAddress.Address,
		TitleLink:  titleLink,
		Title:      title,
		Text:       text,
		Fallback:   fmt.Sprintf("[%s](%s): %s", title, titleLink, text),
	}
}

func (processor *notificationProcessor) newEventSlackAttachment(n *remote.Notification, timezone string) *model.SlackAttachment {
	sa := processor.newSlackAttachment(n)
	sa.Title = "(신규) " + sa.Title

	fields := eventToFields(n.Event, timezone)
	for _, k := range notificationFieldOrder {
		v := fields[k]

		sa.Fields = append(sa.Fields, &model.SlackAttachmentField{
			Title: k,
			Value: strings.Join(v.Strings(), ", "),
			Short: true,
		})
	}

	if n.Event.ResponseRequested && !n.Event.IsOrganizer {
		sa.Actions = NewPostActionForEventResponse(n.Event.ID, n.Event.ResponseStatus.Response, processor.actionURL(config.PathRespond))
	}
	return sa
}

func (processor *notificationProcessor) updatedEventSlackAttachment(n *remote.Notification, prior *remote.Event, timezone string) (bool, *model.SlackAttachment) {
	sa := processor.newSlackAttachment(n)
	sa.Title = "(업데이트됨) " + sa.Title

	newFields := eventToFields(n.Event, timezone)
	priorFields := eventToFields(prior, timezone)
	changed, added, updated, deleted := fields.Diff(priorFields, newFields)
	if !changed {
		return false, nil
	}

	var allChanges []string
	allChanges = append(allChanges, added...)
	allChanges = append(allChanges, updated...)
	allChanges = append(allChanges, deleted...)

	hasImportantChanges := false
	for _, k := range allChanges {
		if isImportantChange(k) {
			hasImportantChanges = true
			break
		}
	}

	if !hasImportantChanges {
		return false, nil
	}

	for _, k := range added {
		if !isImportantChange(k) {
			continue
		}
		sa.Fields = append(sa.Fields, &model.SlackAttachmentField{
			Title: k,
			Value: views.MarkdownToHTMLEntities(strings.Join(newFields[k].Strings(), ", ")),
			Short: true,
		})
	}
	for _, k := range updated {
		if !isImportantChange(k) {
			continue
		}
		sa.Fields = append(sa.Fields, &model.SlackAttachmentField{
			Title: k,
			Value: fmt.Sprintf("~~%s~~ \u2192 %s", views.MarkdownToHTMLEntities(strings.Join(priorFields[k].Strings(), ", ")), views.MarkdownToHTMLEntities(strings.Join(newFields[k].Strings(), ", "))),
			Short: true,
		})
	}
	for _, k := range deleted {
		if !isImportantChange(k) {
			continue
		}
		sa.Fields = append(sa.Fields, &model.SlackAttachmentField{
			Title: k,
			Value: fmt.Sprintf("~~%s~~", views.MarkdownToHTMLEntities(strings.Join(priorFields[k].Strings(), ", "))),
			Short: true,
		})
	}

	if n.Event.ResponseRequested && !n.Event.IsOrganizer && !n.Event.IsCancelled {
		sa.Actions = NewPostActionForEventResponse(n.Event.ID, n.Event.ResponseStatus.Response, processor.actionURL(config.PathRespond))
	}
	return true, sa
}

func isImportantChange(fieldName string) bool {
	for _, ic := range importantNotificationChanges {
		if ic == fieldName {
			return true
		}
	}
	return false
}

func (processor *notificationProcessor) actionURL(action string) string {
	return fmt.Sprintf("%s%s%s", processor.Config.PluginURLPath, config.PathPostAction, action)
}

func NewPostActionForEventResponse(eventID, response, url string) []*model.PostAction {
	context := map[string]interface{}{
		config.EventIDKey: eventID,
	}

	pa := &model.PostAction{
		Name: "응답",
		Type: model.PostActionTypeSelect,
		Integration: &model.PostActionIntegration{
			URL:     url,
			Context: context,
		},
	}

	for _, o := range []string{OptionNotResponded, OptionYes, OptionNo, OptionMaybe} {
		pa.Options = append(pa.Options, &model.PostActionOptions{Text: o, Value: o})
	}
	switch response {
	case ResponseNone:
		pa.DefaultOption = OptionNotResponded
	case ResponseYes:
		pa.DefaultOption = OptionYes
	case ResponseNo:
		pa.DefaultOption = OptionNo
	case ResponseMaybe:
		pa.DefaultOption = OptionMaybe
	}
	return []*model.PostAction{pa}
}

func eventToFields(e *remote.Event, timezone string) fields.Fields {
	date := func(dtStart, dtEnd *remote.DateTime) (time.Time, time.Time, string) {
		if dtStart == nil || dtEnd == nil {
			return time.Time{}, time.Time{}, "해당 없음"
		}

		dtStart = dtStart.In(timezone)
		dtEnd = dtEnd.In(timezone)
		tStart := dtStart.Time()
		tEnd := dtEnd.Time()
		startFormat := "Monday, June 02"
		if tStart.Year() != time.Now().Year() {
			startFormat = "Monday, June 02, 2025"
		}
		startFormat += " · (" + time.Kitchen
		endFormat := " - " + time.Kitchen + ")"
		return tStart, tEnd, tStart.Format(startFormat) + tEnd.Format(endFormat)
	}

	start, end, formattedDate := date(e.Start, e.End)

	minutes := int(end.Sub(start).Round(time.Minute).Minutes())
	hours := int(end.Sub(start).Hours())
	minutes -= hours * 60
	days := int(end.Sub(start).Hours()) / 24
	hours -= days * 24

	dur := ""
	switch {
	case days > 0:
		dur = fmt.Sprintf("%v일", days)

	case e.IsAllDay:
		dur = "종일"

	default:
		switch hours {
		case 0:
			// ignore
		case 1:
			dur = "1시간"
		default:
			dur = fmt.Sprintf("%v시간", hours)
		}
		if minutes > 0 {
			if dur != "" {
				dur += ", "
			}
			dur += fmt.Sprintf("%v분", minutes)
		}
	}

	attendees := []fields.Value{}
	for _, a := range e.Attendees {
		attendees = append(attendees, fields.NewStringValue(
			fmt.Sprintf("[%s](mailto:%s)",
				a.EmailAddress.Name, a.EmailAddress.Address)))
	}

	if len(attendees) == 0 {
		attendees = append(attendees, fields.NewStringValue("없음"))
	}

	ff := fields.Fields{
		FieldSubject:     fields.NewStringValue(views.EnsureSubject(e.Subject)),
		FieldBodyPreview: fields.NewStringValue(valueOrNotDefined(e.BodyPreview)),
		FieldImportance:  fields.NewStringValue(valueOrNotDefined(e.Importance)),
		FieldWhen:        fields.NewStringValue(valueOrNotDefined(formattedDate)),
		FieldDuration:    fields.NewStringValue(valueOrNotDefined(dur)),
		FieldOrganizer: fields.NewStringValue(
			fmt.Sprintf("[%s](mailto:%s)",
				e.Organizer.EmailAddress.Name, e.Organizer.EmailAddress.Address)),
		FieldLocation:       fields.NewStringValue(valueOrNotDefined(e.Location.DisplayName)),
		FieldResponseStatus: fields.NewStringValue(e.ResponseStatus.Response),
		FieldAttendees:      fields.NewMultiValue(attendees...),
	}

	return ff
}

func valueOrNotDefined(s string) string {
	if s == "" {
		return "정의되지 않음"
	}

	return s
}
