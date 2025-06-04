// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	pluginapilicense "github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/command"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/engine"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
)

// Handler handles commands
type Command struct {
	Engine    engine.Engine
	Context   *plugin.Context
	Args      *model.CommandArgs
	Config    *config.Config
	ChannelID string
}

func getNotConnectedText(pluginURL string) string {
	return fmt.Sprintf(
		"Mattermost 계정이 %s 계정에 연결되지 않은 것 같습니다. [계정을 연결하려면 여기를 클릭하세요](%s/oauth2/connect) 또는 `/%s connect` 명령어를 사용하세요.",
		config.Provider.DisplayName,
		pluginURL,
		config.Provider.CommandTrigger,
	)
}

type handleFunc func(parameters ...string) (string, bool, error)

var cmds = []*model.AutocompleteData{
	model.NewAutocompleteData("connect", "", fmt.Sprintf("%s 계정에 연결", config.Provider.DisplayName)),
	model.NewAutocompleteData("disconnect", "", fmt.Sprintf("%s 계정 연결 해제", config.Provider.DisplayName)),
	{ // Summary
		Trigger:  "summary",
		HelpText: "오늘의 일정을 보거나 일일 요약 설정을 편집합니다.",
		SubCommands: []*model.AutocompleteData{
			model.NewAutocompleteData("view", "", "일일 요약 보기."),
			model.NewAutocompleteData("today", "", "오늘의 일정 표시."),
			model.NewAutocompleteData("tomorrow", "", "내일의 일정 표시."),
			model.NewAutocompleteData("settings", "", "일일 요약 설정 보기."),
			model.NewAutocompleteData("time", "", "일일 요약을 받을 시간 설정."),
			model.NewAutocompleteData("enable", "", "일일 요약 활성화."),
			model.NewAutocompleteData("disable", "", "일일 요약 비활성화."),
		},
	},
	model.NewAutocompleteData("viewcal", "", "오늘을 포함한 향후 14일간의 일정 보기."),
	{ // Create
		Trigger:  "event",
		HelpText: "일정 관리.",
		SubCommands: []*model.AutocompleteData{
			model.NewAutocompleteData("create", "", "새 일정 생성 (데스크톱 전용)."),
		},
	},
	model.NewAutocompleteData("today", "", "오늘의 일정 표시."),
	model.NewAutocompleteData("tomorrow", "", "내일의 일정 표시."),
	model.NewAutocompleteData("settings", "", "사용자 개인 설정 편집."),
	model.NewAutocompleteData("info", "", "이 플러그인 버전에 대한 정보 읽기."),
	model.NewAutocompleteData("help", "", "명령어 도움말 텍스트 읽기"),
}

// Register should be called by the plugin to register all necessary commands
func Register(client *pluginapilicense.Client) error {
	names := []string{}
	for _, subCommand := range cmds {
		names = append(names, subCommand.Trigger)
	}

	hint := "[" + strings.Join(names[:4], "|") + "...]"

	cmd := model.NewAutocompleteData(config.Provider.CommandTrigger, hint, fmt.Sprintf("%s 캘린더와 상호작용합니다.", config.Provider.DisplayName))
	cmd.SubCommands = cmds

	iconData, err := command.GetIconData(&client.System, fmt.Sprintf("assets/profile-%s.svg", config.Provider.Name))
	if err != nil {
		return errors.Wrap(err, "아이콘 데이터를 가져오는데 실패했습니다")
	}

	return client.SlashCommand.Register(&model.Command{
		Trigger:              config.Provider.CommandTrigger,
		DisplayName:          config.Provider.DisplayName,
		Description:          fmt.Sprintf("%s 캘린더와 상호작용합니다.", config.Provider.DisplayName),
		AutoComplete:         true,
		AutoCompleteDesc:     strings.Join(names, ", "),
		AutoCompleteHint:     "(하위명령어)",
		AutocompleteData:     cmd,
		AutocompleteIconData: iconData,
	})
}

// Handle should be called by the plugin when a command invocation is received from the Mattermost server.
func (c *Command) Handle() (string, bool, error) {
	cmd, parameters, err := c.isValid()
	if err != nil {
		return "", false, err
	}

	handler := c.help
	switch cmd {
	case "info":
		handler = c.info
	case "connect":
		handler = c.connect
	case "disconnect":
		handler = c.requireConnectedUser(c.disconnect)
	case "summary":
		handler = c.requireConnectedUser(c.dailySummary)
	case "viewcal":
		handler = c.requireConnectedUser(c.viewCalendar)
	case "settings":
		handler = c.requireConnectedUser(c.settings)
	case "events":
		handler = c.requireConnectedUser(c.event)
	// Admin only
	case "showcals":
		handler = c.requireConnectedUser(c.requireAdminUser(c.showCalendars))
	case "avail":
		handler = c.requireConnectedUser(c.requireAdminUser(c.debugAvailability))
	case "subscribe":
		handler = c.requireConnectedUser(c.requireAdminUser(c.subscribe))
	case "unsubscribe":
		handler = c.requireConnectedUser(c.requireAdminUser(c.unsubscribe))
	// Aliases
	case "today":
		parameters = []string{"today"}
		handler = c.requireConnectedUser(c.dailySummary)
	case "tomorrow":
		parameters = []string{"tomorrow"}
		handler = c.requireConnectedUser(c.dailySummary)
	}
	out, mustRedirectToDM, err := handler(parameters...)
	if err != nil {
		return out, false, errors.WithMessagef(err, "명령어 /%s %s 실행에 실패했습니다", config.Provider.CommandTrigger, cmd)
	}

	return out, mustRedirectToDM, nil
}

func (c *Command) isValid() (subcommand string, parameters []string, err error) {
	if c.Context == nil || c.Args == nil {
		return "", nil, errors.New("command.Handler에 잘못된 인수가 전달되었습니다")
	}
	split := strings.Fields(c.Args.Command)
	cmd := split[0]
	if cmd != "/"+config.Provider.CommandTrigger {
		return "", nil, fmt.Errorf("%q는 지원되지 않는 명령어입니다. 시스템 관리자에게 문의하세요", cmd)
	}

	parameters = []string{}
	subcommand = ""
	if len(split) > 1 {
		subcommand = split[1]
	}
	if len(split) > 2 {
		parameters = split[2:]
	}

	return subcommand, parameters, nil
}

func (c *Command) user() *engine.User {
	return engine.NewUser(c.Args.UserId)
}

func (c *Command) requireConnectedUser(handle handleFunc) handleFunc {
	return func(parameters ...string) (string, bool, error) {
		connected, err := c.isConnected()
		if err != nil {
			return "", false, err
		}

		if !connected {
			return getNotConnectedText(c.Config.PluginURL), false, nil
		}
		return handle(parameters...)
	}
}

func (c *Command) requireAdminUser(handle handleFunc) handleFunc {
	return func(parameters ...string) (string, bool, error) {
		authorized, err := c.Engine.IsAuthorizedAdmin(c.Args.UserId)
		if err != nil {
			return "", false, err
		}
		if !authorized {
			return "권한이 없습니다", false, nil
		}

		return handle(parameters...)
	}
}

func (c *Command) isConnected() (bool, error) {
	_, err := c.Engine.GetRemoteUser(c.Args.UserId)
	if err == store.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}
