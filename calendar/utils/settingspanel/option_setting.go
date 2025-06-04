// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package settingspanel

import (
	"errors"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
)

type optionSetting struct {
	store         SettingStore
	title         string
	description   string
	id            string
	dependsOn     string
	defaultOption string
	options       []string
}

func NewOptionSetting(id, title, description, dependsOn, defaultOption string, options []string, store SettingStore) Setting {
	return &optionSetting{
		title:         title,
		description:   description,
		id:            id,
		dependsOn:     dependsOn,
		options:       options,
		store:         store,
		defaultOption: defaultOption,
	}
}

func (s *optionSetting) Set(userID string, value interface{}) error {
	err := s.store.SetSetting(userID, s.id, value)
	if err != nil {
		return err
	}

	return nil
}

func (s *optionSetting) Get(userID string) (interface{}, error) {
	value, err := s.store.GetSetting(userID, s.id)
	if err != nil {
		return "", err
	}
	valueString, ok := value.(string)
	if !ok {
		return "", errors.New("현재 값이 문자열이 아닙니다")
	}

	return valueString, nil
}

func (s *optionSetting) GetID() string {
	return s.id
}

func (s *optionSetting) GetTitle() string {
	return s.title
}

func (s *optionSetting) GetDescription() string {
	return s.description
}

func (s *optionSetting) GetDependency() string {
	return s.dependsOn
}

func (s *optionSetting) GetSlackAttachments(userID, settingHandler string, disabled bool) (*model.SlackAttachment, error) {
	title := fmt.Sprintf("설정: %s", s.title)
	currentValueMessage := "비활성화됨"

	actions := []*model.PostAction{}
	if !disabled {
		currentTextValue, err := s.Get(userID)
		if err != nil {
			return nil, err
		}

		if currentTextValue == "" {
			currentTextValue = s.defaultOption
		}

		currentValueMessage = fmt.Sprintf("**현재 값:** %s", currentTextValue)

		actionOptions := model.PostAction{
			Name: "옵션 선택:",
			Integration: &model.PostActionIntegration{
				URL: settingHandler + "?" + s.id + "=true",
				Context: map[string]interface{}{
					ContextIDKey: s.id,
				},
			},
			Type:    "select",
			Options: stringsToOptions(s.options),
		}

		actions = []*model.PostAction{&actionOptions}
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

func (s *optionSetting) IsDisabled(foreignValue interface{}) bool {
	return foreignValue == "false"
}
