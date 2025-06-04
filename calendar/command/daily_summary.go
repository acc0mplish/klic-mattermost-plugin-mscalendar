// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
)

func getDailySummaryHelp() string {
	return "### 일일 요약 명령어:\n" +
		fmt.Sprintf("`/%s summary view` - 일일 요약 보기\n", config.Provider.CommandTrigger) +
		fmt.Sprintf("`/%s summary settings` - 일일 요약 설정 보기\n", config.Provider.CommandTrigger) +
		fmt.Sprintf("`/%s summary time 8:00AM` - 일일 요약을 받을 시간 설정\n", config.Provider.CommandTrigger) +
		fmt.Sprintf("`/%s summary enable` - 일일 요약 활성화\n", config.Provider.CommandTrigger) +
		fmt.Sprintf("`/%s summary disable` - 일일 요약 비활성화", config.Provider.CommandTrigger)
}

func getDailySummarySetTimeErrorMessage() string {
	return fmt.Sprintf("시간을 입력해주세요. 예시:\n`/%s summary time 8:00AM`", config.Provider.CommandTrigger)
}

func (c *Command) dailySummary(parameters ...string) (string, bool, error) {
	if len(parameters) == 0 {
		return getDailySummaryHelp(), false, nil
	}

	switch parameters[0] {
	case "view", "today":
		postStr, err := c.Engine.GetDaySummaryForUser(time.Now(), c.user())
		if err != nil {
			if strings.Contains(err.Error(), store.ErrorRefreshTokenNotSet) || strings.Contains(err.Error(), store.ErrorUserInactive) {
				return store.ErrorUserInactive, false, nil
			}

			return err.Error(), false, err
		}
		return postStr, false, nil
	case "tomorrow":
		postStr, err := c.Engine.GetDaySummaryForUser(time.Now().Add(time.Hour*24), c.user())
		if err != nil {
			if strings.Contains(err.Error(), store.ErrorRefreshTokenNotSet) || strings.Contains(err.Error(), store.ErrorUserInactive) {
				return store.ErrorUserInactive, false, nil
			}

			return err.Error(), false, err
		}
		return postStr, false, nil
	case "time":
		if len(parameters) != 2 {
			return getDailySummarySetTimeErrorMessage(), false, nil
		}
		val := parameters[1]

		dsum, err := c.Engine.SetDailySummaryPostTime(c.user(), val)
		if err != nil {
			if strings.Contains(err.Error(), store.ErrorRefreshTokenNotSet) || strings.Contains(err.Error(), store.ErrorUserInactive) {
				return store.ErrorUserInactive, false, nil
			}

			return err.Error() + "\n" + getDailySummarySetTimeErrorMessage(), false, nil
		}

		return dailySummaryResponse(dsum), false, nil
	case "settings":
		dsum, err := c.Engine.GetDailySummarySettingsForUser(c.user())
		if err != nil {
			if strings.Contains(err.Error(), store.ErrorRefreshTokenNotSet) || strings.Contains(err.Error(), store.ErrorUserInactive) {
				return store.ErrorUserInactive, false, nil
			}

			return err.Error() + "\n아래 명령어를 사용하여 일일 요약을 설정해야 할 수 있습니다.\n" + getDailySummaryHelp(), false, nil
		}

		return dailySummaryResponse(dsum), false, nil
	case "enable":
		dsum, err := c.Engine.SetDailySummaryEnabled(c.user(), true)
		if err != nil {
			return err.Error(), false, err
		}

		return dailySummaryResponse(dsum), false, nil
	case "disable":
		dsum, err := c.Engine.SetDailySummaryEnabled(c.user(), false)
		if err != nil {
			return err.Error(), false, err
		}
		return dailySummaryResponse(dsum), false, nil
	}
	return "잘못된 명령어입니다. 다시 시도해주세요\n\n" + getDailySummaryHelp(), false, nil
}

func dailySummaryResponse(dsum *store.DailySummaryUserSettings) string {
	if dsum.PostTime == "" {
		return "일일 요약 시간이 아직 설정되지 않았습니다.\n" + getDailySummarySetTimeErrorMessage()
	}

	enableStr := ""
	if !dsum.Enable {
		enableStr = fmt.Sprintf(", 하지만 비활성화되어 있습니다. `/%s summary enable`로 활성화할 수 있습니다", config.Provider.CommandTrigger)
	}
	return fmt.Sprintf("일일 요약이 %s %s에 표시되도록 설정되어 있습니다%s.", dsum.PostTime, dsum.Timezone, enableStr)
}
