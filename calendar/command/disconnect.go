// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

func (c *Command) disconnect(_ ...string) (string, bool, error) {
	err := c.Engine.DisconnectUser(c.Args.UserId)
	if err != nil {
		return "", false, err
	}
	c.Engine.ClearSettingsPosts(c.Args.UserId)

	return "계정 연결이 성공적으로 해제되었습니다", false, nil
}
