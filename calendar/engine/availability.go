// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"fmt"
	"sort"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/engine/views"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/remote"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/bot"
)

const (
	calendarViewTimeWindowSize    = 10 * time.Minute
	StatusSyncJobInterval         = 5 * time.Minute
	upcomingEventNotificationTime = 10 * time.Minute

	upcomingEventNotificationWindow = (StatusSyncJobInterval * 11) / 10 // 110% of the interval
	logTruncateMsg                  = "메시지가 너무 많아 로그를 잘랐습니다"
	logTruncateLimit                = 5
)

var (
	errNoUsersNeedToBeSynced = errors.New("동기화가 필요한 사용자가 없습니다")
)

type StatusSyncJobSummary struct {
	NumberOfUsersFailedStatusChanged int
	NumberOfUsersStatusChanged       int
	NumberOfUsersProcessed           int
}

type Availability interface {
	GetCalendarViews(users []*store.User) ([]*remote.ViewCalendarResponse, error)
	Sync(mattermostUserID string) (string, *StatusSyncJobSummary, error)
	SyncAll() (string, *StatusSyncJobSummary, error)
}

func (m *mscalendar) Sync(mattermostUserID string) (string, *StatusSyncJobSummary, error) {
	user, err := m.Store.LoadUserFromIndex(mattermostUserID)
	if err != nil {
		return "", nil, err
	}

	userIndex := store.UserIndex{user}

	err = m.Filter(withSuperuserClient)
	if err != nil && !errors.Is(err, remote.ErrSuperUserClientNotSupported) {
		return "", &StatusSyncJobSummary{}, errors.Wrap(err, "슈퍼유저 클라이언트를 필터링할 수 없습니다")
	}

	return m.syncUsers(userIndex, errors.Is(err, remote.ErrSuperUserClientNotSupported))
}

func (m *mscalendar) SyncAll() (string, *StatusSyncJobSummary, error) {
	userIndex, err := m.Store.LoadUserIndex()
	if err != nil {
		if err.Error() == "not found" {
			return "사용자 인덱스에서 사용자를 찾을 수 없습니다", &StatusSyncJobSummary{}, nil
		}
		return "", &StatusSyncJobSummary{}, errors.Wrap(err, "사용자 인덱스에서 사용자를 로드할 수 없습니다")
	}

	err = m.Filter(withSuperuserClient)
	if err != nil && !errors.Is(err, remote.ErrSuperUserClientNotSupported) {
		return "", &StatusSyncJobSummary{}, errors.Wrap(err, "슈퍼유저 클라이언트를 필터링할 수 없습니다")
	}

	result, jobSummary, err := m.syncUsers(userIndex, errors.Is(err, remote.ErrSuperUserClientNotSupported))
	if result != "" && err != nil {
		return result, jobSummary, nil
	}

	return result, jobSummary, err
}

// retrieveUsersToSync는 동기화하고 알림을 보낼 사용자와 해당 캘린더 데이터를 검색합니다
// fetchIndividually 매개변수는 사용자를 반복하는 동안 캘린더 데이터를 가져올지
// (개별 자격 증명 사용) 아니면 반복 후 일괄적으로 가져올지를 결정합니다.

func (m *mscalendar) retrieveUsersToSync(userIndex store.UserIndex, syncJobSummary *StatusSyncJobSummary, fetchIndividually bool) ([]*store.User, []*remote.ViewCalendarResponse, error) {
	start := time.Now().UTC()
	end := time.Now().UTC().Add(calendarViewTimeWindowSize)

	numberOfLogs := 0
	users := []*store.User{}
	calendarViews := []*remote.ViewCalendarResponse{}
	for _, u := range userIndex {
		// TODO fetch users from kvstore in batches, and process in batches instead of all at once
		user, err := m.Store.LoadUser(u.MattermostUserID)
		if err != nil {
			syncJobSummary.NumberOfUsersFailedStatusChanged++
			if numberOfLogs < logTruncateLimit {
				m.Logger.Warnf("사용자 인덱스에서 사용자 %s를 로드할 수 없습니다. err=%v", u.MattermostUserID, err)
			} else if numberOfLogs == logTruncateLimit {
				m.Logger.Warnf(logTruncateMsg)
			}
			numberOfLogs++

			// In case of error in loading, skip this user and continue with the next user
			continue
		}

		// If user does not have the proper features enabled, just go to the next one
		if !(user.IsConfiguredForStatusUpdates() || user.IsConfiguredForCustomStatusUpdates() || user.Settings.ReceiveReminders) {
			continue
		}

		if fetchIndividually {
			engine, err := m.FilterCopy(withActingUser(user.MattermostUserID))
			if err != nil {
				m.Logger.Warnf("사용자 인덱스에서 활성 사용자 %s를 활성화할 수 없습니다. err=%v", user.MattermostUserID, err)
				continue
			}

			calendarUser := newUserFromStoredUser(user)
			calendarEvents, err := engine.GetCalendarEvents(calendarUser, start, end, true)
			if err != nil {
				syncJobSummary.NumberOfUsersFailedStatusChanged++
				m.Logger.With(bot.LogContext{
					"user": u.MattermostUserID,
					"err":  err,
				}).Warnf("캘린더 이벤트를 가져올 수 없습니다")
				continue
			}

			calendarViews = append(calendarViews, calendarEvents)
		}

		users = append(users, user)
	}

	if len(users) == 0 {
		return users, calendarViews, errNoUsersNeedToBeSynced
	}

	if !fetchIndividually {
		var err error
		calendarViews, err = m.GetCalendarViews(users)
		if err != nil {
			return users, calendarViews, errors.Wrap(err, "연결된 사용자의 캘린더 뷰를 가져올 수 없습니다")
		}
	}

	if len(calendarViews) == 0 {
		return users, calendarViews, fmt.Errorf("캘린더 뷰를 찾을 수 없습니다")
	}

	// Sort events for all fetched calendar views
	for _, view := range calendarViews {
		events := view.Events
		sort.Slice(events, func(i, j int) bool {
			return events[i].Start.Time().UnixMicro() < events[j].Start.Time().UnixMicro()
		})
	}

	return users, calendarViews, nil
}

func (m *mscalendar) syncUsers(userIndex store.UserIndex, fetchIndividually bool) (string, *StatusSyncJobSummary, error) {
	syncJobSummary := &StatusSyncJobSummary{}
	if len(userIndex) == 0 {
		return "연결된 사용자를 찾을 수 없습니다", syncJobSummary, nil
	}
	syncJobSummary.NumberOfUsersProcessed = len(userIndex)

	users, calendarViews, err := m.retrieveUsersToSync(userIndex, syncJobSummary, fetchIndividually)
	if err != nil {
		return err.Error(), syncJobSummary, errors.Wrapf(err, "동기화할 사용자를 검색하는 중 오류 발생 (individually=%v)", fetchIndividually)
	}

	m.deliverReminders(users, calendarViews, fetchIndividually)
	out, numberOfUsersStatusChanged, numberOfUsersFailedStatusChanged, err := m.setUserStatuses(users, calendarViews)
	if err != nil {
		return "", syncJobSummary, errors.Wrap(err, "사용자 상태를 설정하는 중 오류 발생")
	}

	syncJobSummary.NumberOfUsersFailedStatusChanged += numberOfUsersFailedStatusChanged
	syncJobSummary.NumberOfUsersStatusChanged = numberOfUsersStatusChanged

	return out, syncJobSummary, nil
}

func (m *mscalendar) deliverReminders(users []*store.User, calendarViews []*remote.ViewCalendarResponse, fetchIndividually bool) {
	numberOfLogs := 0
	toNotify := []*store.User{}
	for _, u := range users {
		if u.Settings.ReceiveReminders {
			toNotify = append(toNotify, u)
		}
	}
	if len(toNotify) == 0 {
		return
	}

	usersByRemoteID := map[string]*store.User{}
	for _, u := range toNotify {
		usersByRemoteID[u.Remote.ID] = u
	}

	for _, view := range calendarViews {
		user, ok := usersByRemoteID[view.RemoteUserID]
		if !ok {
			continue
		}
		if view.Error != nil {
			if numberOfLogs < logTruncateLimit {
				m.Logger.Warnf("%s의 가용성을 가져오는 중 오류 발생. err=%s", user.MattermostUserID, view.Error.Message)
			} else if numberOfLogs == logTruncateLimit {
				m.Logger.Warnf(logTruncateMsg)
			}
			numberOfLogs++
			continue
		}

		mattermostUserID := usersByRemoteID[view.RemoteUserID].MattermostUserID
		if fetchIndividually {
			engine, err := m.FilterCopy(withActingUser(user.MattermostUserID))
			if err != nil {
				m.Logger.With(bot.LogContext{"err": err}).Errorf("사용자 엔진을 가져오는 중 오류 발생")
				continue
			}
			engine.notifyUpcomingEvents(mattermostUserID, view.Events)
		} else {
			m.notifyUpcomingEvents(mattermostUserID, view.Events)
		}
	}
}

func (m *mscalendar) setUserStatuses(users []*store.User, calendarViews []*remote.ViewCalendarResponse) (string, int, int, error) {
	numberOfLogs, numberOfUserStatusChange, numberOfUserErrorInStatusChange := 0, 0, 0
	toUpdate := []*store.User{}
	for _, u := range users {
		if u.IsConfiguredForStatusUpdates() || u.IsConfiguredForCustomStatusUpdates() {
			toUpdate = append(toUpdate, u)
		}
	}
	if len(toUpdate) == 0 {
		return "상태 업데이트를 원하는 사용자가 없습니다", numberOfUserStatusChange, numberOfUserErrorInStatusChange, nil
	}

	mattermostUserIDs := []string{}
	usersByRemoteID := map[string]*store.User{}
	for _, u := range toUpdate {
		mattermostUserIDs = append(mattermostUserIDs, u.MattermostUserID)
		usersByRemoteID[u.Remote.ID] = u
	}

	statuses, appErr := m.PluginAPI.GetMattermostUserStatusesByIds(mattermostUserIDs)
	if appErr != nil {
		return "", numberOfUserStatusChange, numberOfUserErrorInStatusChange, errors.Wrap(appErr, "연결된 사용자의 Mattermost 사용자 상태를 가져오는 중 오류 발생")
	}
	statusMap := map[string]*model.Status{}
	for _, s := range statuses {
		statusMap[s.UserId] = s
	}

	var res string
	for _, view := range calendarViews {
		isStatusChanged := false
		user, ok := usersByRemoteID[view.RemoteUserID]
		if !ok {
			continue
		}
		if view.Error != nil {
			if numberOfLogs < logTruncateLimit {
				m.Logger.Warnf("%s의 가용성을 가져오는 중 오류 발생. err=%s", user.MattermostUserID, view.Error.Message)
			} else if numberOfLogs == logTruncateLimit {
				m.Logger.Warnf(logTruncateMsg)
			}
			numberOfLogs++
			numberOfUserErrorInStatusChange++
			continue
		}

		mattermostUserID := usersByRemoteID[view.RemoteUserID].MattermostUserID
		status, ok := statusMap[mattermostUserID]
		if !ok {
			continue
		}

		events := filterBusyAndAttendeeEvents(view.Events)
		events = getMergedEvents(events)

		var err error
		if user.IsConfiguredForStatusUpdates() {
			res, isStatusChanged, err = m.setStatusFromCalendarView(user, status, events)
			if err != nil {
				if numberOfLogs < logTruncateLimit {
					m.Logger.Warnf("사용자 %s 상태 설정 중 오류 발생. err=%v", user.MattermostUserID, err)
				} else if numberOfLogs == logTruncateLimit {
					m.Logger.Warnf(logTruncateMsg)
				}
				numberOfLogs++
				numberOfUserErrorInStatusChange++
			}
			if isStatusChanged {
				numberOfUserStatusChange++
			}
		}

		if user.IsConfiguredForCustomStatusUpdates() {
			res, isStatusChanged, err = m.setCustomStatusFromCalendarView(user, events)
			if err != nil {
				if numberOfLogs < logTruncateLimit {
					m.Logger.Warnf("사용자 %s 커스텀 상태 설정 중 오류 발생. err=%v", user.MattermostUserID, err)
				} else if numberOfLogs == logTruncateLimit {
					m.Logger.Warnf(logTruncateMsg)
				}
				numberOfLogs++
				numberOfUserErrorInStatusChange++
			}

			// Increment count only when we have not updated the status of the user from the options to have status change count per user.
			if isStatusChanged && user.Settings.UpdateStatusFromOptions == store.NotSetStatusOption {
				numberOfUserStatusChange++
			}
		}
	}

	if res != "" {
		return res, numberOfUserStatusChange, numberOfUserErrorInStatusChange, nil
	}

	return utils.JSONBlock(calendarViews), numberOfUserStatusChange, numberOfUserErrorInStatusChange, nil
}

func (m *mscalendar) setCustomStatusFromCalendarView(user *store.User, events []*remote.Event) (string, bool, error) {
	isStatusChanged := false
	if !user.IsConfiguredForCustomStatusUpdates() {
		return "사용자가 커스텀 상태 설정을 원하지 않습니다", isStatusChanged, nil
	}

	if len(events) == 0 {
		if user.IsCustomStatusSet {
			if err := m.PluginAPI.RemoveMattermostUserCustomStatus(user.MattermostUserID); err != nil {
				m.Logger.Warnf("사용자 %s 커스텀 상태 제거 중 오류 발생. err=%v", user.MattermostUserID, err)
			}

			if err := m.Store.StoreUserCustomStatusUpdates(user.MattermostUserID, false); err != nil {
				return "", isStatusChanged, err
			}
		}

		return "커스텀 상태를 설정할 이벤트가 없습니다", isStatusChanged, nil
	}

	currentUser, err := m.PluginAPI.GetMattermostUser(user.MattermostUserID)
	if err != nil {
		return "", isStatusChanged, err
	}

	currentCustomStatus := currentUser.GetCustomStatus()
	if currentCustomStatus != nil && !user.IsCustomStatusSet {
		return "사용자가 이미 커스텀 상태를 설정했습니다. 커스텀 상태 변경을 무시합니다", isStatusChanged, nil
	}

	if appErr := m.PluginAPI.UpdateMattermostUserCustomStatus(user.MattermostUserID, &model.CustomStatus{
		Emoji:     "calendar",
		Text:      "회의 중",
		ExpiresAt: events[0].End.Time(),
		Duration:  "date_and_time",
	}); appErr != nil {
		return "", isStatusChanged, appErr
	}

	isStatusChanged = true
	if err := m.Store.StoreUserCustomStatusUpdates(user.MattermostUserID, true); err != nil {
		return "", isStatusChanged, err
	}

	return "", isStatusChanged, nil
}

func (m *mscalendar) setStatusFromCalendarView(user *store.User, status *model.Status, events []*remote.Event) (string, bool, error) {
	isStatusChanged := false
	currentStatus := status.Status
	if !user.IsConfiguredForStatusUpdates() {
		return "상태 업데이트 옵션에서 설정된 값이 없습니다", isStatusChanged, nil
	}

	if currentStatus == model.StatusOffline && !user.Settings.GetConfirmation {
		return "사용자가 오프라인이고 상태 변경 확인을 원하지 않습니다. 상태 변경 없음", isStatusChanged, nil
	}

	busyStatus := model.StatusDnd
	if user.Settings.UpdateStatusFromOptions == store.AwayStatusOption {
		busyStatus = model.StatusAway
	}

	if len(user.ActiveEvents) == 0 && len(events) == 0 {
		return "로컬 또는 원격에 이벤트가 없습니다. 상태 변경 없음.", isStatusChanged, nil
	}

	if len(user.ActiveEvents) > 0 && len(events) == 0 {
		message := fmt.Sprintf("사용자가 더 이상 캘린더에서 바쁘지 않지만 바쁨(%s)으로 설정되지 않았습니다. 상태 변경 없음.", busyStatus)
		if currentStatus == busyStatus {
			message = "사용자가 더 이상 캘린더에서 바쁘지 않습니다. 상태를 온라인으로 설정합니다."
			if user.LastStatus != "" {
				message = fmt.Sprintf("사용자가 더 이상 캘린더에서 바쁘지 않습니다. 상태를 이전 상태(%s)로 설정합니다", user.LastStatus)
			}
			err := m.setStatusOrAskUser(user, status, events, true)
			if err != nil {
				return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 사용자 상태 설정 중 오류 발생", user.MattermostUserID)
			}
			isStatusChanged = true
		}

		err := m.Store.StoreUserActiveEvents(user.MattermostUserID, []string{})
		if err != nil {
			return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 활성 이벤트 저장 중 오류 발생", user.MattermostUserID)
		}
		return message, isStatusChanged, nil
	}

	remoteHashes := []string{}
	for _, e := range events {
		if e.IsCancelled {
			continue
		}
		h := fmt.Sprintf("%s %s", e.ICalUID, e.Start.Time().UTC().Format(time.RFC3339))
		remoteHashes = append(remoteHashes, h)
	}

	if len(user.ActiveEvents) == 0 {
		var err error
		if currentStatus == busyStatus {
			user.LastStatus = ""
			if status.Manual {
				user.LastStatus = currentStatus
			}
			m.Store.StoreUser(user)
			err = m.Store.StoreUserActiveEvents(user.MattermostUserID, remoteHashes)
			if err != nil {
				return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 활성 이벤트 저장 중 오류 발생", user.MattermostUserID)
			}
			return "사용자가 이미 바쁨으로 표시되었습니다. 상태 변경 없음.", isStatusChanged, nil
		}
		err = m.setStatusOrAskUser(user, status, events, false)
		if err != nil {
			return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 사용자 상태 설정 중 오류 발생", user.MattermostUserID)
		}
		isStatusChanged = true
		err = m.Store.StoreUserActiveEvents(user.MattermostUserID, remoteHashes)
		if err != nil {
			return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 활성 이벤트 저장 중 오류 발생", user.MattermostUserID)
		}
		return fmt.Sprintf("사용자가 한가했지만 지금은 바쁩니다(%s). 상태를 바쁨으로 설정합니다.", busyStatus), isStatusChanged, nil
	}

	newEventExists := false
	for _, r := range remoteHashes {
		found := false
		for _, loc := range user.ActiveEvents {
			if loc == r {
				found = true
				break
			}
		}
		if !found {
			newEventExists = true
			break
		}
	}

	if !newEventExists {
		return fmt.Sprintf("활성 이벤트에 변경 사항이 없습니다. 총 이벤트 수: %d", len(events)), isStatusChanged, nil
	}

	message := "사용자가 이미 바쁩니다. 상태 변경 없음."
	if currentStatus != busyStatus {
		err := m.setStatusOrAskUser(user, status, events, false)
		if err != nil {
			return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 사용자 상태 설정 중 오류 발생", user.MattermostUserID)
		}
		isStatusChanged = true
		message = fmt.Sprintf("사용자가 한가했지만 지금은 바쁩니다. 상태를 바쁨(%s)으로 설정합니다.", busyStatus)
	}

	err := m.Store.StoreUserActiveEvents(user.MattermostUserID, remoteHashes)
	if err != nil {
		return "", isStatusChanged, errors.Wrapf(err, "사용자 %s의 활성 이벤트 저장 중 오류 발생", user.MattermostUserID)
	}

	return message, isStatusChanged, nil
}

// setStatusOrAskUser to which status change, and whether it should update the status automatically or ask the user.
// - user: the user to change the status. We use user.LastStatus to determine the status the user had before the beginning of the meeting.
// - currentStatus: currentStatus, to decide whether to store this status when the user is free. This gets assigned to user.LastStatus at the beginning of the meeting.
// - events: the list of events that are triggering this status change
// - isFree: whether the user is free or busy, to decide to which status to change
func (m *mscalendar) setStatusOrAskUser(user *store.User, currentStatus *model.Status, events []*remote.Event, isFree bool) error {
	toSet := model.StatusOnline
	if isFree && user.LastStatus != "" {
		toSet = user.LastStatus
		user.LastStatus = ""
	}

	if !isFree {
		toSet = model.StatusDnd
		if user.Settings.UpdateStatusFromOptions == store.AwayStatusOption {
			toSet = model.StatusAway
		}
		if !user.Settings.GetConfirmation {
			user.LastStatus = ""
			if currentStatus.Manual {
				user.LastStatus = currentStatus.Status
			}
		}
	}

	err := m.Store.StoreUser(user)
	if err != nil {
		return err
	}

	if !user.Settings.GetConfirmation {
		_, appErr := m.PluginAPI.UpdateMattermostUserStatus(user.MattermostUserID, toSet)
		if appErr != nil {
			return appErr
		}
		return nil
	}

	url := fmt.Sprintf("%s%s%s", m.Config.PluginURLPath, config.PathPostAction, config.PathConfirmStatusChange)
	_, err = m.Poster.DMWithAttachments(user.MattermostUserID, views.RenderStatusChangeNotificationView(events, toSet, url))
	if err != nil {
		return err
	}
	return nil
}

func (m *mscalendar) GetCalendarEvents(user *User, start, end time.Time, excludeDeclined bool) (*remote.ViewCalendarResponse, error) {
	err := m.Filter(withClient)
	if err != nil {
		return nil, errors.Wrap(err, "GetCalendarEvents에서 withClient 오류")
	}

	events, err := m.client.GetEventsBetweenDates(user.Remote.ID, start, end)
	if err != nil {
		return nil, errors.Wrapf(err, "사용자 %s의 이벤트를 가져오는 중 오류 발생", user.MattermostUserID)
	}

	if excludeDeclined {
		events = m.excludeDeclinedEvents(events)
	}

	return &remote.ViewCalendarResponse{
		RemoteUserID: user.Remote.ID,
		Events:       events,
	}, nil
}

func (m *mscalendar) GetCalendarViews(users []*store.User) ([]*remote.ViewCalendarResponse, error) {
	err := m.Filter(withClient)
	if err != nil {
		return nil, fmt.Errorf("GetCalendarViews에서 withClient 오류: %w", err)
	}

	start := time.Now().UTC()
	end := time.Now().UTC().Add(calendarViewTimeWindowSize)

	params := []*remote.ViewCalendarParams{}
	for _, u := range users {
		params = append(params, &remote.ViewCalendarParams{
			RemoteUserID: u.Remote.ID,
			StartTime:    start,
			EndTime:      end,
		})
	}

	return m.client.DoBatchViewCalendarRequests(params)
}

func (m *mscalendar) notifyUpcomingEvents(mattermostUserID string, events []*remote.Event) {
	var timezone string
	for _, event := range events {
		if event.IsCancelled {
			continue
		}
		upcomingTime := time.Now().Add(upcomingEventNotificationTime)
		start := event.Start.Time()
		diff := start.Sub(upcomingTime)

		if (diff < upcomingEventNotificationWindow) && (diff > -upcomingEventNotificationWindow) {
			var err error
			if timezone == "" {
				timezone, err = m.GetTimezoneByID(mattermostUserID)
				if err != nil {
					m.Logger.Warnf("notifyUpcomingEvents 시간대 가져오기 오류. err=%v", err)
					return
				}
			}

			_, attachment, err := views.RenderUpcomingEventAsAttachment(event, timezone)
			if err != nil {
				m.Logger.Warnf("notifyUpcomingEvent 일정 항목 렌더링 오류. err=%v", err)
				continue
			}

			_, err = m.Poster.DMWithAttachments(mattermostUserID, attachment)
			if err != nil {
				m.Logger.Warnf("notifyUpcomingEvents DM 생성 오류. err=%v", err)
				continue
			}

			// Process channel reminders
			eventMetadata, errMetadata := m.Store.LoadEventMetadata(event.ICalUID)
			if errMetadata != nil && !errors.Is(errMetadata, store.ErrNotFound) {
				m.Logger.With(bot.LogContext{
					"eventID": event.ID,
					"err":     errMetadata.Error(),
				}).Warnf("notifyUpcomingEvents 채널 알림을 위한 스토어 확인 오류")
				continue
			}

			if eventMetadata != nil {
				for channelID := range eventMetadata.LinkedChannelIDs {
					post := &model.Post{
						ChannelId: channelID,
						Message:   "예정된 이벤트",
					}
					attachment, errRender := views.RenderEventAsAttachment(event, timezone, views.ShowTimezoneOption(timezone))
					if errRender != nil {
						m.Logger.With(bot.LogContext{"err": errRender}).Errorf("notifyUpcomingEvents 채널 게시물 렌더링 오류")
						continue
					}
					model.ParseSlackAttachment(post, []*model.SlackAttachment{attachment})
					errPoster := m.Poster.CreatePost(post)
					if errPoster != nil {
						m.Logger.With(bot.LogContext{"err": errPoster}).Warnf("notifyUpcomingEvents 채널에 게시물 생성 오류")
						continue
					}
				}
			}
		}
	}
}

func filterBusyAndAttendeeEvents(events []*remote.Event) []*remote.Event {
	result := []*remote.Event{}
	for _, e := range events {
		// Not setting custom status for events without attendees since those are unlikely to be meetings.
		if e.ShowAs == "busy" && !e.IsCancelled && len(e.Attendees) >= 1 {
			result = append(result, e)
		}
	}
	return result
}

// getMergedEvents accepts a sorted array of events, and returns events after merging them, if overlapping or if the meeting duration is less than StatusSyncJobInterval.
func getMergedEvents(events []*remote.Event) []*remote.Event {
	if len(events) <= 1 {
		return events
	}

	idx := 0
	for i := 1; i < len(events); i++ {
		if areEventsMergeable(events[idx], events[i]) {
			events[idx].End = events[i].End
		} else {
			idx++
			events[idx] = events[i]
		}
	}

	return events[0 : idx+1]
}

/*
areEventsMergeable 함수는 두 이벤트를 하나의 이벤트로 병합할 수 있는지 확인합니다.
이 함수에서 확인하는 두 가지 조건이 있습니다:
  - 두 이벤트가 겹치는 경우, event1의 종료 시간이 event2의 시작 시간보다
    크거나 같으면 이러한 이벤트들을 하나의 이벤트로 병합할 수 있습니다.
    예: event1: 1:01–1:04, event2: 1:03–1:05. 최종 이벤트: 1:01–1:05.
  - event1 종료 시간과 event1 시작 시간의 차이가 StatusSyncJobInterval보다 작거나 같고, event2 시작 시간과 event1 종료 시간의 차이가 StatusSyncJobInterval보다 작거나 같은 경우. 이는 StatusSyncJobInterval 시간 범위 내에서 발생하는 이벤트들을 병합하기 위해 수행됩니다.
    예: event1: 1:01–1:02, event2: 1:03–1:05, StatusSyncJobInterval: 5분. 최종 이벤트: 1:01–1:05.
    이는 작업이 5분마다 실행될 때 두 이벤트가 단일 API 호출에서 함께 가져오므로 event2를 건너뛰는 것을 방지하기 위해 수행됩니다.
*/

func areEventsMergeable(event1, event2 *remote.Event) bool {
	return (event1.End.Time().UnixMicro() >= event2.Start.Time().UnixMicro()) || (event1.End.Time().Sub(event1.Start.Time()) <= StatusSyncJobInterval && event2.Start.Time().Sub(event1.End.Time()) <= StatusSyncJobInterval)
}

