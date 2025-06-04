// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

func (c *Command) unsubscribe(_ ...string) (string, bool, error) {
	_, err := c.Engine.LoadMyEventSubscription()
	if err != nil {
		return "이벤트를 구독하고 있지 않습니다.", false, nil
	}

	err = c.Engine.DeleteMyEventSubscription()
	if err != nil {
		return "", false, err
	}

	return "이벤트 구독을 해제했습니다.", false, nil
}
