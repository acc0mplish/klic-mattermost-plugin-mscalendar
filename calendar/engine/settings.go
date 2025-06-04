// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/bot"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/settingspanel"
)

type Settings interface {
	PrintSettings(userID string)
	ClearSettingsPosts(userID string)
}

func (m *mscalendar) PrintSettings(userID string) {
	m.SettingsPanel.Print(userID)
}

func (m *mscalendar) ClearSettingsPosts(userID string) {
	err := m.SettingsPanel.Clear(userID)
	if err != nil {
		m.Logger.Warnf("설정 게시물 지우기 중 오류 발생. err=%v", err)
	}
}

func NewSettingsPanel(bot bot.Bot, panelStore settingspanel.PanelStore, settingStore settingspanel.SettingStore, settingsHandler, pluginURL string, getCal func(userID string) Engine, providerFeatures config.ProviderFeatures) settingspanel.Panel {
	settings := []settingspanel.Setting{}
	settings = append(settings, settingspanel.NewOptionSetting(
		store.UpdateStatusFromOptionsSettingID,
		"상태 업데이트",
		"회의 중일 때 Mattermost에서 상태를 업데이트하시겠습니까?",
		"",
		store.NotSetStatusOption,
		[]string{store.AwayStatusOption, store.DNDStatusOption, store.NotSetStatusOption},
		settingStore,
	))
	settings = append(settings, settingspanel.NewBoolSetting(
		store.GetConfirmationSettingID,
		"확인 받기",
		"상태를 자동으로 업데이트하기 전에 확인을 받으시겠습니까?",
		store.UpdateStatusFromOptionsSettingID,
		settingStore,
	))
	settings = append(settings, settingspanel.NewBoolSetting(
		store.SetCustomStatusSettingID,
		"사용자 지정 상태 설정",
		"회의 중일 때 Mattermost에서 사용자 지정 상태를 자동으로 설정하시겠습니까?",
		"",
		settingStore,
	))
	settings = append(settings, settingspanel.NewBoolSetting(
		store.ReceiveRemindersSettingID,
		"알림 받기",
		"다가오는 이벤트에 대한 알림을 받으시겠습니까?",
		"",
		settingStore,
	))
	if providerFeatures.EventNotifications {
		settings = append(settings, NewNotificationsSetting(getCal))
	}
	settings = append(settings, NewDailySummarySetting(
		settingStore,
		func(userID string) (string, error) { return getCal(userID).GetTimezone(NewUser(userID)) },
	))
	return settingspanel.NewSettingsPanel(settings, bot, bot, panelStore, settingsHandler, pluginURL)
}
