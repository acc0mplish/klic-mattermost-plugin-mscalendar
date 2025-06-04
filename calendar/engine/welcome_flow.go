// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/config"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/bot"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/utils/flow"
)

type WelcomeFlow struct {
	controller       bot.FlowController
	onFlowDone       func(userID string)
	url              string
	providerFeatures config.ProviderFeatures
	steps            []flow.Step
}

func NewWelcomeFlow(bot bot.FlowController, welcomer Welcomer, providerFeatures config.ProviderFeatures) *WelcomeFlow {
	wf := WelcomeFlow{
		url:              "/welcome",
		controller:       bot,
		onFlowDone:       welcomer.WelcomeFlowEnd,
		providerFeatures: providerFeatures,
	}
	wf.makeSteps()
	return &wf
}

func (wf *WelcomeFlow) Step(i int) flow.Step {
	if i < 0 {
		return nil
	}
	if i >= len(wf.steps) {
		return nil
	}
	return wf.steps[i]
}

func (wf *WelcomeFlow) URL() string {
	return wf.url
}

func (wf *WelcomeFlow) Length() int {
	return len(wf.steps)
}

func (wf *WelcomeFlow) StepDone(userID string, step int, value bool) {
	wf.controller.NextStep(userID, step, value)
}

func (wf *WelcomeFlow) FlowDone(userID string) {
	wf.onFlowDone(userID)
}

func (wf *WelcomeFlow) makeSteps() {
	steps := []flow.Step{
		&flow.EmptyStep{
			Title:   "상태 업데이트",
			Message: fmt.Sprintf("회의 중일 때 상태를 \"자리 비움\" 또는 \"방해 금지\"로 업데이트하도록 플러그인을 구성하려면 `/%s`를 입력하세요.", config.Provider.CommandTrigger),
		},
		&flow.SimpleStep{
			Title:                "사용자 정의 상태 설정",
			Message:              "회의 중일 때 Mattermost 사용자 정의 상태를 자동으로 설정하시겠습니까?",
			PropertyName:         store.SetCustomStatusPropertyName,
			TrueButtonMessage:    "예 - 자동으로 Mattermost 사용자 정의 상태를 :calendar:로 설정",
			FalseButtonMessage:   "아니요, 사용자 정의 상태를 설정하지 않음",
			TrueResponseMessage:  "회의 중일 때 자동으로 Mattermost 사용자 정의 상태를 설정합니다.",
			FalseResponseMessage: "회의 중일 때 Mattermost 사용자 정의 상태를 설정하지 않습니다.",
		},
		// &flow.SimpleStep{
		// 	Title:                "상태 변경 확인",
		// 	Message:              "각 이벤트에 대해 상태를 업데이트하기 전에 확인을 받으시겠습니까?",
		// 	PropertyName:         store.GetConfirmationPropertyName,
		// 	TrueButtonMessage:    "예 - 확인을 받고 싶습니다",
		// 	FalseButtonMessage:   "아니요 - 자동으로 상태를 업데이트",
		// 	TrueResponseMessage:  "좋습니다. 상태를 업데이트하기 전에 확인을 보내드리겠습니다.",
		// 	FalseResponseMessage: "좋습니다. 확인 없이 자동으로 상태를 업데이트하겠습니다.",
		// },
		// &flow.SimpleStep{
		// 	Title:                "회의 중 상태",
		// 	Message:              "회의 중일 때 상태를 `자리 비움`으로 설정하시겠습니까, 아니면 `방해 금지`로 설정하시겠습니까? `방해 금지`로 설정하면 알림이 무음됩니다.",
		// 	PropertyName:         store.ReceiveNotificationsDuringMeetingName,
		// 	TrueButtonMessage:    "자리 비움",
		// 	FalseButtonMessage:   "방해 금지",
		// 	TrueResponseMessage:  "좋습니다. 상태가 자리 비움으로 설정됩니다.",
		// 	FalseResponseMessage: "좋습니다. 상태가 방해 금지로 설정됩니다.",
		// },
	}

	if wf.providerFeatures.EventNotifications {
		steps = append(steps, &flow.SimpleStep{
			Title:                "이벤트 구독",
			Message:              "이벤트에 초대받을 때 알림을 받으시겠습니까?",
			PropertyName:         store.SubscribePropertyName,
			TrueButtonMessage:    "예 - 새 이벤트에 대한 알림을 받고 싶습니다",
			FalseButtonMessage:   "아니요 - 새 이벤트 알림을 받지 않겠습니다",
			TrueResponseMessage:  "좋습니다. 새 이벤트를 받을 때마다 메시지를 받게 됩니다.",
			FalseResponseMessage: "좋습니다. 새 이벤트에 대한 알림을 받지 않습니다.",
		})
	}

	steps = append(steps, &flow.SimpleStep{
		Title:                "미리 알림 받기",
		Message:              "예정된 이벤트에 대한 미리 알림을 받으시겠습니까?",
		PropertyName:         store.ReceiveUpcomingEventReminderName,
		TrueButtonMessage:    "예 - 예정된 이벤트에 대한 미리 알림을 받고 싶습니다",
		FalseButtonMessage:   "아니요 - 예정된 이벤트 알림을 받지 않겠습니다",
		TrueResponseMessage:  "좋습니다. 회의 전에 메시지를 받게 됩니다.",
		FalseResponseMessage: "좋습니다. 예정된 이벤트에 대한 알림을 받지 않습니다.",
	}, &flow.EmptyStep{
		Title:   "일일 요약",
		Message: fmt.Sprintf("`/%s summary time 8:00AM`을 입력하거나 `/%s settings`를 사용하여 설정에 액세스하여 일일 요약을 설정할 수 있습니다.", config.Provider.CommandTrigger, config.Provider.CommandTrigger),
	})

	wf.steps = steps
}
