// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/settingspanel"
)

type notificationSetting struct {
	getCal      func(string) Engine
	title       string
	description string
	id          string
	dependsOn   string
}

func NewNotificationsSetting(getCal func(string) Engine) settingspanel.Setting {
	return &notificationSetting{
		title:       "새 이벤트 알림 받기",
		description: "새 이벤트를 구독하고 이벤트가 생성될 때 메시지를 받으시겠습니까?",
		id:          "new_or_updated_event_setting",
		dependsOn:   "",
		getCal:      getCal,
	}
}

func (s *notificationSetting) Set(userID string, value interface{}) error {
	boolValue := false
	if value == "true" {
		boolValue = true
	}

	cal := s.getCal(userID)

	if boolValue {
		_, err := cal.LoadMyEventSubscription()
		if err != nil {
			_, err := cal.CreateMyEventSubscription()
			if err != nil {
				return err
			}
		}

		return nil
	}

	_, err := cal.LoadMyEventSubscription()
	if err == nil {
		return cal.DeleteMyEventSubscription()
	}
	return nil
}

func (s *notificationSetting) Get(userID string) (interface{}, error) {
	cal := s.getCal(userID)
	_, err := cal.LoadMyEventSubscription()
	if err == nil {
		return "true", nil
	}

	return "false", nil
}

func (s *notificationSetting) GetID() string {
	return s.id
}

func (s *notificationSetting) GetTitle() string {
	return s.title
}

func (s *notificationSetting) GetDescription() string {
	return s.description
}

func (s *notificationSetting) GetDependency() string {
	return s.dependsOn
}

func (s *notificationSetting) getActionStyle(actionValue, currentValue string) string {
	if actionValue == currentValue {
		return "primary"
	}
	return "default"
}

func (s *notificationSetting) GetSlackAttachments(userID, settingHandler string, disabled bool) (*model.SlackAttachment, error) {
	title := fmt.Sprintf("설정: %s", s.title)
	currentValueMessage := "비활성화됨"

	actions := []*model.PostAction{}
	if !disabled {
		currentValue, err := s.Get(userID)
		if err != nil {
			return nil, err
		}

		currentTextValue := "아니오"
		if currentValue == "true" {
			currentTextValue = "예"
		}
		currentValueMessage = fmt.Sprintf("**현재 값:** %s", currentTextValue)

		actionTrue := model.PostAction{
			Name:  "예",
			Style: s.getActionStyle("true", currentValue.(string)),
			Integration: &model.PostActionIntegration{
				URL: settingHandler,
				Context: map[string]interface{}{
					settingspanel.ContextIDKey:          s.id,
					settingspanel.ContextButtonValueKey: "true",
				},
			},
		}

		actionFalse := model.PostAction{
			Name:  "아니오",
			Style: s.getActionStyle("false", currentValue.(string)),
			Integration: &model.PostActionIntegration{
				URL: settingHandler,
				Context: map[string]interface{}{
					settingspanel.ContextIDKey:          s.id,
					settingspanel.ContextButtonValueKey: "false",
				},
			},
		}
		actions = []*model.PostAction{&actionTrue, &actionFalse}
	}

	text := fmt.Sprintf("%s\n%s", s.description, currentValueMessage)
	sa := model.SlackAttachment{
		Title:    title,
		Text:     text,
		Actions:  actions,
		Fallback: fmt.Sprintf("%s: %s", title, text),
	}

	return &sa, nil
}

func (s *notificationSetting) IsDisabled(foreignValue interface{}) bool {
	return foreignValue == "false"
}
