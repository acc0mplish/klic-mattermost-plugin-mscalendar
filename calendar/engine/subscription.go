// Copyright (c) 2019-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package engine

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/remote"
	"github.com/mattermost/mattermost-plugin-mscalendar/calendar/store"
)

type Subscriptions interface {
	CreateMyEventSubscription() (*store.Subscription, error)
	RenewMyEventSubscription() (*store.Subscription, error)
	DeleteOrphanedSubscription(*store.Subscription) error
	DeleteMyEventSubscription() error
	ListRemoteSubscriptions() ([]*remote.Subscription, error)
	LoadMyEventSubscription() (*store.Subscription, error)
}

func (m *mscalendar) CreateMyEventSubscription() (*store.Subscription, error) {
	err := m.Filter(withClient)
	if err != nil {
		return nil, fmt.Errorf("CreateMyEventSubscription에서 withClient 오류: %w", err)
	}

	sub, err := m.client.CreateMySubscription(m.Config.GetNotificationURL(), m.actingUser.Remote.ID)
	if err != nil {
		return nil, err
	}

	storedSub := &store.Subscription{
		Remote:              sub,
		MattermostCreatorID: m.actingUser.MattermostUserID,
		PluginVersion:       m.Config.PluginVersion,
	}
	err = m.Store.StoreUserSubscription(m.actingUser.User, storedSub)
	if err != nil {
		return nil, err
	}

	return storedSub, nil
}

func (m *mscalendar) LoadMyEventSubscription() (*store.Subscription, error) {
	err := m.Filter(withActingUserExpanded)
	if err != nil {
		return nil, err
	}

	// TODO: m.actingUser.Settings.EventSubscriptionID가 비어있으면 구독이 없음

	storedSub, err := m.Store.LoadSubscription(m.actingUser.Settings.EventSubscriptionID)
	if err != nil {
		return nil, err
	}
	return storedSub, err
}

func (m *mscalendar) ListRemoteSubscriptions() ([]*remote.Subscription, error) {
	err := m.Filter(withClient)
	if err != nil {
		return nil, fmt.Errorf("ListRemoteSubscriptions에서 withClient 오류: %w", err)
	}
	subs, err := m.client.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	return subs, nil
}

func (m *mscalendar) RenewMyEventSubscription() (*store.Subscription, error) {
	err := m.Filter(withClient)
	if err != nil {
		return nil, fmt.Errorf("RenewMyEventSubscription에서 withClient 오류: %w", err)
	}

	subscriptionID := m.actingUser.Settings.EventSubscriptionID
	if subscriptionID == "" {
		return nil, nil
	}

	sub, err := m.Store.LoadSubscription(subscriptionID)
	if err != nil {
		return nil, errors.Wrap(err, "구독 로드 오류")
	}

	renewed, err := m.client.RenewSubscription(m.Config.GetNotificationURL(), m.actingUser.Remote.ID, sub.Remote)
	if err != nil {
		if strings.Contains(err.Error(), "The object was not found") {
			err = m.Store.DeleteUserSubscription(m.actingUser.User, subscriptionID)
			if err != nil {
				return nil, err
			}

			m.Logger.Infof("Mattermost 사용자 %s의 구독 %s가 만료되었습니다. 새 구독을 생성합니다.", m.actingUser.MattermostUserID, subscriptionID)
			return m.CreateMyEventSubscription()
		}
		return nil, err
	}

	storedSub, err := m.Store.LoadSubscription(m.actingUser.Settings.EventSubscriptionID)
	if err != nil {
		return nil, err
	}
	storedSub.Remote = renewed

	err = m.Store.StoreUserSubscription(m.actingUser.User, storedSub)
	if err != nil {
		return nil, err
	}
	return storedSub, err
}

func (m *mscalendar) DeleteMyEventSubscription() error {
	err := m.Filter(withActingUserExpanded)
	if err != nil {
		return err
	}

	subscriptionID := m.actingUser.Settings.EventSubscriptionID

	sub, err := m.Store.LoadSubscription(subscriptionID)
	if err != nil {
		return errors.Wrap(err, "구독 로드 오류")
	}

	err = m.DeleteOrphanedSubscription(sub)
	if err != nil {
		return err
	}

	err = m.Store.DeleteUserSubscription(m.actingUser.User, subscriptionID)
	if err != nil {
		return errors.WithMessagef(err, "구독 %s 삭제 실패", subscriptionID)
	}

	return nil
}

func (m *mscalendar) DeleteOrphanedSubscription(sub *store.Subscription) error {
	err := m.Filter(withClient)
	if err != nil {
		return fmt.Errorf("DeleteOrphanedSubscription에서 withClient 오류: %w", err)
	}
	err = m.client.DeleteSubscription(sub.Remote)
	if err != nil {
		return errors.WithMessagef(err, "구독 %s 삭제 실패", sub.Remote.ID)
	}
	return nil
}
