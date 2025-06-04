// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/oauth2connect"
)

const BotWelcomeMessage = "봇 사용자가 계정 %s에 연결되었습니다."

const (
	RemoteUserAlreadyConnected         = "%s 계정 `%s`이(가) 이미 Mattermost 계정 `%s`에 연결되어 있습니다. 해당 Mattermost 계정으로 로그인한 후 `/%s disconnect`를 실행해 주세요"
	RemoteUserAlreadyConnectedDisabled = "%s 계정 `%s`이(가) 이미 Mattermost 계정에 연결되어 있지만, 해당 계정이 비활성화되어 있습니다. 계정을 활성화하고 다른 Mattermost 계정으로 로그인한 후 `/%s disconnect`를 실행한 다음 다시 시도해 주세요"
	RemoteUserAlreadyConnectedNotFound = "%s 계정 `%s`이(가) 이미 Mattermost 계정에 연결되어 있지만, 해당 Mattermost 사용자를 찾을 수 없습니다"
)

type oauth2App struct {
	Env
}

func NewOAuth2App(env Env) oauth2connect.App {
	return &oauth2App{
		Env: env,
	}
}

func (app *oauth2App) InitOAuth2(mattermostUserID string) (url string, err error) {
	user, err := app.Store.LoadUser(mattermostUserID)
	if err == nil {
		return "", fmt.Errorf("사용자가 이미 %s에 연결되어 있습니다", user.Remote.Mail)
	}

	conf := app.Remote.NewOAuth2Config()
	state := fmt.Sprintf("%v_%v", model.NewId()[0:15], mattermostUserID)
	err = app.Store.StoreOAuth2State(state)
	if err != nil {
		return "", err
	}

	return conf.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

func (app *oauth2App) CompleteOAuth2(authedUserID, code, state string) error {
	if authedUserID == "" || code == "" || state == "" {
		return errors.New("사용자, 코드 또는 상태가 누락되었습니다")
	}

	oconf := app.Remote.NewOAuth2Config()

	err := app.Store.VerifyOAuth2State(state)
	if err != nil {
		return errors.WithMessage(err, "저장된 상태가 누락되었습니다")
	}

	mattermostUserID := strings.Split(state, "_")[1]
	if mattermostUserID != authedUserID {
		return errors.New("권한이 없습니다, 사용자 ID가 일치하지 않습니다")
	}

	ctx := context.Background()
	tok, err := oconf.Exchange(ctx, code)
	if err != nil {
		return err
	}

	client := app.Remote.MakeUserClient(ctx, tok, mattermostUserID, app.Poster, app.Store)
	me, err := client.GetMe()
	if err != nil {
		return err
	}

	uid, err := app.Store.LoadMattermostUserID(me.ID)
	if err == nil {
		user, userErr := app.PluginAPI.GetMattermostUser(uid)
		if userErr == nil {
			msg := fmt.Sprintf(RemoteUserAlreadyConnected, config.Provider.DisplayName, me.Mail, user.Username, config.Provider.CommandTrigger)
			app.Poster.DM(authedUserID, msg)
			return errors.New(msg)
		}

		if userErr == store.ErrNotFound {
			msg := fmt.Sprintf(RemoteUserAlreadyConnectedDisabled, config.Provider.DisplayName, me.Mail, config.Provider.CommandTrigger)
			app.Poster.DM(authedUserID, msg)
			return errors.New(msg)
		}

		// 연결된 MM 계정을 가져올 수 없습니다. 연결 시도를 거부합니다.
		msg := fmt.Sprintf(RemoteUserAlreadyConnectedNotFound, config.Provider.DisplayName, me.Mail)
		app.Poster.DM(authedUserID, msg)
		return errors.New(msg)
	}

	user, userErr := app.PluginAPI.GetMattermostUser(mattermostUserID)
	if userErr != nil {
		return fmt.Errorf("mattermost 사용자 검색 중 오류 발생 (%s): %w", mattermostUserID, userErr)
	}

	u := &store.User{
		PluginVersion:         app.Config.PluginVersion,
		MattermostUserID:      mattermostUserID,
		MattermostUsername:    user.Username,
		MattermostDisplayName: user.GetDisplayName(model.ShowFullName),
		Remote:                me,
		OAuth2Token:           tok,
		Settings:              store.DefaultSettings,
	}

	mailboxSettings, err := client.GetMailboxSettings(me.ID)
	if err != nil {
		return err
	}

	u.Settings.DailySummary = &store.DailySummaryUserSettings{
		PostTime: "8:00AM",
		Timezone: mailboxSettings.TimeZone,
		Enable:   false,
	}

	err = app.Store.StoreUser(u)
	if err != nil {
		return err
	}

	err = app.Store.StoreUserInIndex(u)
	if err != nil {
		return err
	}

	app.Welcomer.AfterSuccessfullyConnect(mattermostUserID, me.Mail)

	return nil
}
