// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils"
)

func (c *Command) subscribe(parameters ...string) (string, bool, error) {
	if len(parameters) > 0 && parameters[0] == "list" {
		return c.debugList()
	}

	_, err := c.Engine.LoadMyEventSubscription()
	if err == nil {
		return "이미 이벤트를 구독하고 있습니다.", false, nil
	}

	_, err = c.Engine.CreateMyEventSubscription()
	if err != nil {
		return "", false, err
	}
	return "이제 이벤트를 구독합니다.", false, nil
}

func (c *Command) debugList() (string, bool, error) {
	subs, err := c.Engine.ListRemoteSubscriptions()
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("구독:%s", utils.JSONBlock(subs)), false, nil
}
