package metrics

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/Azure/ARO-HCP/internal/api"
	"github.com/Azure/ARO-HCP/internal/api/arm"
	"github.com/Azure/ARO-HCP/internal/database"
	"github.com/Azure/ARO-HCP/internal/mocks"
)

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

func TestSubscriptionCollector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nosubs := slices.Values([]*database.SubscriptionDocument{})
	subs := slices.Values([]*database.SubscriptionDocument{
		database.NewSubscriptionDocument(
			"00000000-0000-0000-0000-000000000000",
			&arm.Subscription{
				State:            arm.SubscriptionStateRegistered,
				RegistrationDate: api.Ptr(time.Now().String()),
			}),
	})
	ctrl := gomock.NewController(t)

	mockDBClient := mocks.NewMockDBClient(ctrl)

	r := prometheus.NewPedanticRegistry()
	collector := NewSubscriptionCollector(r, mockDBClient, "test")

	t.Run("no subscription", func(t *testing.T) {
		mockIter := mocks.NewMockDBClientIterator[database.SubscriptionDocument](ctrl)
		mockIter.EXPECT().
			Items(gomock.Any()).
			Return(database.DBClientIteratorItem[database.SubscriptionDocument](nosubs))
		mockIter.EXPECT().
			GetError().
			Return(nil)

		mockDBClient.EXPECT().
			ListAllSubscriptionDocs().
			Return(mockIter).
			Times(1)
		collector.refresh(logger)

		assertMetrics(t, r, 5, `# HELP frontend_subscription_collector_failed_syncs_total Total number of failed syncs for the Subscription collector.
# TYPE frontend_subscription_collector_failed_syncs_total counter
frontend_subscription_collector_failed_syncs_total 0
# HELP frontend_subscription_collector_syncs_total Total number of syncs for the Subscription collector.
# TYPE frontend_subscription_collector_syncs_total counter
frontend_subscription_collector_syncs_total 1
# HELP frontend_subscription_collector_last_sync Last sync operation's result (1: success, 0: failed).
# TYPE frontend_subscription_collector_last_sync gauge
frontend_subscription_collector_last_sync 1
`)
	})

	t.Run("db error", func(t *testing.T) {
		mockIter := mocks.NewMockDBClientIterator[database.SubscriptionDocument](ctrl)
		mockIter.EXPECT().
			Items(gomock.Any()).
			Return(database.DBClientIteratorItem[database.SubscriptionDocument](nosubs))
		mockIter.EXPECT().
			GetError().
			Return(errors.New("db error"))
		mockDBClient.EXPECT().
			ListAllSubscriptionDocs().
			Return(mockIter).
			Times(1)

		collector.refresh(logger)

		assertMetrics(t, r, 5, `# HELP frontend_subscription_collector_failed_syncs_total Total number of failed syncs for the Subscription collector.
# TYPE frontend_subscription_collector_failed_syncs_total counter
frontend_subscription_collector_failed_syncs_total 1
# HELP frontend_subscription_collector_syncs_total Total number of syncs for the Subscription collector.
# TYPE frontend_subscription_collector_syncs_total counter
frontend_subscription_collector_syncs_total 2
# HELP frontend_subscription_collector_last_sync Last sync operation's result (1: success, 0: failed).
# TYPE frontend_subscription_collector_last_sync gauge
frontend_subscription_collector_last_sync 0
`)
	})

	t.Run("refresh with 1 subscription", func(t *testing.T) {
		mockIter := mocks.NewMockDBClientIterator[database.SubscriptionDocument](ctrl)
		mockIter.EXPECT().
			Items(gomock.Any()).
			Return(database.DBClientIteratorItem[database.SubscriptionDocument](subs))
		mockIter.EXPECT().
			GetError().
			Return(nil)
		mockDBClient.EXPECT().
			ListAllSubscriptionDocs().
			Return(mockIter).
			Times(1)

		collector.refresh(logger)

		assertMetrics(t, r, 7, `
# HELP frontend_lifecycle_last_update_timestamp_seconds Reports the timestamp when the subscription has been updated for the last time.
# TYPE frontend_lifecycle_last_update_timestamp_seconds gauge
frontend_lifecycle_last_update_timestamp_seconds{location="test",subscription_id="00000000-0000-0000-0000-000000000000"} 0
# HELP frontend_lifecycle_state Reports the current state of the subscription.
# TYPE frontend_lifecycle_state gauge
frontend_lifecycle_state{location="test",state="Registered",subscription_id="00000000-0000-0000-0000-000000000000"} 1
# HELP frontend_subscription_collector_failed_syncs_total Total number of failed syncs for the Subscription collector.
# TYPE frontend_subscription_collector_failed_syncs_total counter
frontend_subscription_collector_failed_syncs_total 1
# HELP frontend_subscription_collector_syncs_total Total number of syncs for the Subscription collector.
# TYPE frontend_subscription_collector_syncs_total counter
frontend_subscription_collector_syncs_total 3
# HELP frontend_subscription_collector_last_sync Last sync operation's result (1: success, 0: failed).
# TYPE frontend_subscription_collector_last_sync gauge
frontend_subscription_collector_last_sync 1
`)
	})
}

func assertMetrics(t *testing.T, r prometheus.Gatherer, metrics int, expectedOutput string) {
	t.Helper()

	n, err := testutil.GatherAndCount(r)
	assert.NoError(t, err)
	assert.Equal(t, metrics, n)

	// We can't check the timestamp-based metrics.
	err = testutil.GatherAndCompare(
		r,
		bytes.NewBufferString(expectedOutput),
		errCounterName,
		refreshCounterName,
		lastSyncResultName,
		subscriptionStateName,
		subscriptionLastUpdatedName,
	)
	assert.NoError(t, err)

	problems, err := testutil.GatherAndLint(r)
	assert.NoError(t, err)

	for _, p := range problems {
		t.Errorf("metric %q: %s", p.Metric, p.Text)
	}
}
