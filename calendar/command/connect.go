// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
)

const (
	ConnectBotAlreadyConnectedTemplate = "봇 계정이 이미 %s 계정 `%s`에 연결되어 있습니다. 다른 계정에 연결하려면 먼저 `/%s disconnect_bot`을 실행하세요."
	ConnectBotSuccessTemplate          = "[봇의 %s 계정을 연결하려면 여기를 클릭하세요.](%s/oauth2/connect_bot)"
	ConnectAlreadyConnectedTemplate    = "귀하의 Mattermost 계정이 이미 %s 계정 `%s`에 연결되어 있습니다. 다른 계정에 연결하려면 먼저 `/%s disconnect`를 실행하세요."
	ConnectErrorMessage                = "연결을 시도하는 중에 문제가 발생했습니다. err="
)

func (c *Command) connect(_ ...string) (string, bool, error) {
	ru, err := c.Engine.GetRemoteUser(c.Args.UserId)
	if err == nil {
		return fmt.Sprintf(ConnectAlreadyConnectedTemplate, config.Provider.DisplayName, ru.Mail, config.Provider.CommandTrigger), false, nil
	}

	out := ""

	err = c.Engine.Welcome(c.Args.UserId)
	if err != nil {
		out = ConnectErrorMessage + err.Error()
	}

	return out, true, nil
}
