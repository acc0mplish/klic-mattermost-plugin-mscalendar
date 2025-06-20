// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"time"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/remote"
)

type Calendar interface {
	CreateCalendar(user *User, calendar *remote.Calendar) (*remote.Calendar, error)
	CreateEvent(user *User, event *remote.Event, mattermostUserIDs []string) (*remote.Event, error)
	DeleteCalendar(user *User, calendarID string) error
	FindMeetingTimes(user *User, meetingParams *remote.FindMeetingTimesParameters) (*remote.MeetingTimeSuggestionResults, error)
	GetCalendars(user *User) ([]*remote.Calendar, error)
	ViewCalendar(user *User, from, to time.Time) ([]*remote.Event, error)
}

func (m *mscalendar) ViewCalendar(user *User, from, to time.Time) ([]*remote.Event, error) {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return nil, err
	}
	return m.client.GetDefaultCalendarView(user.Remote.ID, from, to)
}

func (m *mscalendar) getTodayCalendarEvents(user *User, now time.Time, timezone string) ([]*remote.Event, error) {
	err := m.Filter(
		withClient,
	)
	if err != nil {
		return nil, err
	}

	err = m.ExpandRemoteUser(user)
	if err != nil {
		return nil, err
	}

	from, to := getTodayHoursForTimezone(now, timezone)
	return m.client.GetDefaultCalendarView(user.Remote.ID, from, to)
}

func (m *mscalendar) excludeDeclinedEvents(events []*remote.Event) (result []*remote.Event) {
	for ix, evt := range events {
		if evt.ResponseStatus == nil || evt.ResponseStatus.Response != remote.EventResponseStatusDeclined {
			result = append(result, events[ix])
		}
	}

	return
}

func (m *mscalendar) CreateCalendar(user *User, calendar *remote.Calendar) (*remote.Calendar, error) {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return nil, err
	}
	return m.client.CreateCalendar(user.Remote.ID, calendar)
}

func (m *mscalendar) CreateEvent(user *User, event *remote.Event, mattermostUserIDs []string) (*remote.Event, error) {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return nil, err
	}

	// 매핑되지 않은 Mattermost 사용자 초대
	for id := range mattermostUserIDs {
		mattermostUserID := mattermostUserIDs[id]
		_, err := m.Store.LoadUser(mattermostUserID)
		if err != nil {
			if err.Error() == "not found" {
				_, err = m.Poster.DM(mattermostUserID, "당신은 %s 이벤트에 초대되었지만 계정을 연결하지 않았습니다. `/%s connect`를 사용하여 %s 계정을 연결하고 자유롭게 참여하세요", m.Provider.DisplayName, m.Provider.CommandTrigger, m.Provider.DisplayName)
				if err != nil {
					m.Logger.Warnf("CreateEvent DM 생성 오류. err=%v", err)
					continue
				}
			}
		}
	}

	return m.client.CreateEvent(user.Remote.ID, event)
}

func (m *mscalendar) DeleteCalendar(user *User, calendarID string) error {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return err
	}

	return m.client.DeleteCalendar(user.Remote.ID, calendarID)
}

func (m *mscalendar) FindMeetingTimes(user *User, meetingParams *remote.FindMeetingTimesParameters) (*remote.MeetingTimeSuggestionResults, error) {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return nil, err
	}

	return m.client.FindMeetingTimes(user.Remote.ID, meetingParams)
}

func (m *mscalendar) GetCalendars(user *User) ([]*remote.Calendar, error) {
	err := m.Filter(
		withClient,
		withUserExpanded(user),
	)
	if err != nil {
		return nil, err
	}

	return m.client.GetCalendars(user.Remote.ID)
}
