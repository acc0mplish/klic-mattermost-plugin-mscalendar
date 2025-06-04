// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

func (c *Command) event(parameters ...string) (string, bool, error) {
	if len(parameters) == 0 {
		return getDailySummaryHelp(), false, nil
	}

	if parameters[0] == "create" {
		return "이벤트 생성은 데스크톱에서만 지원됩니다.", false, nil
	}

	return "", false, nil
}
