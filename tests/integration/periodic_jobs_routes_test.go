package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
)

func TestPeriodicJobsRoutes_CRUD_EnableDisable_RunNow(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7311, "user7311")

	list, err := client.PeriodicJobsList(ctx)
	if err != nil {
		t.Fatalf("PeriodicJobsList failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected default maintenance jobs, got %d", len(list))
	}

	foundKinds := map[string]bool{}
	for _, item := range list {
		foundKinds[string(item.Kind)] = true
	}
	if !foundKinds["clean.old_events"] || !foundKinds["clean.pending_files"] {
		t.Fatalf("expected maintenance presets, got %+v", foundKinds)
	}
	if !foundKinds["clean.stale_uploads"] {
		t.Fatalf("expected clean.stale_uploads preset, got %+v", foundKinds)
	}

	assertMaintenanceRetention := func(jobKind api.PeriodicJobKind, expected string) {
		t.Helper()
		for _, item := range list {
			if item.Kind != jobKind {
				continue
			}
			detail, err := client.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams{ID: item.ID})
			if err != nil {
				t.Fatalf("PeriodicJobsGet %s failed: %v", jobKind, err)
			}
			args, ok := detail.Args.Get()
			if !ok {
				t.Fatalf("expected %s args to be set", jobKind)
			}
			retentionRaw, ok := args["retention"]
			if !ok {
				t.Fatalf("expected retention in %s args: %+v", jobKind, args)
			}
			var retention string
			if err := json.Unmarshal(retentionRaw, &retention); err != nil {
				t.Fatalf("unmarshal retention for %s: %v", jobKind, err)
			}
			if retention != expected {
				t.Fatalf("expected %s retention=%s, got %s", jobKind, expected, retention)
			}
			return
		}
		t.Fatalf("expected maintenance job %s", jobKind)
	}

	assertMaintenanceRetention(api.PeriodicJobKind("clean.old_events"), "5d")
	assertMaintenanceRetention(api.PeriodicJobKind("clean.stale_uploads"), "1d")

	created, err := client.PeriodicJobsCreate(ctx, &api.PeriodicJobCreate{
		Name:           "daily-photos",
		Enabled:        api.NewOptBool(true),
		CronExpression: "*/5 * * * *",
		Args: api.SyncArgs{
			Source:         "local:///tmp/source",
			DestinationDir: "/backup/photos",
			Headers:        api.NewOptSyncArgsHeaders(api.SyncArgsHeaders{}),
			Proxy:          api.NewOptString(""),
			Filters: api.NewOptSyncFilters(api.SyncFilters{
				Include: []string{"**/*.jpg"},
			}),
			Options: api.NewOptSyncOptions(api.SyncOptions{
				PartSize: api.NewOptInt64(32 * 1024 * 1024),
				Sync:     api.NewOptBool(true),
			}),
		},
	})
	if err != nil {
		t.Fatalf("PeriodicJobsCreate failed: %v", err)
	}
	if created.Kind != api.PeriodicJobKindSyncRun {
		t.Fatalf("expected sync.run kind, got %s", created.Kind)
	}

	got, err := client.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams{ID: created.ID})
	if err != nil {
		t.Fatalf("PeriodicJobsGet failed: %v", err)
	}
	if got.Name != "daily-photos" || got.CronExpression != "*/5 * * * *" {
		t.Fatalf("unexpected periodic job payload: %+v", got)
	}

	updated, err := client.PeriodicJobsUpdate(ctx, &api.PeriodicJobUpdate{
		Name:           api.NewOptString("daily-photos-v2"),
		Enabled:        api.NewOptBool(true),
		CronExpression: api.NewOptString("*/10 * * * *"),
		Args: api.NewOptPeriodicJobUpdateArgs(api.PeriodicJobUpdateArgs{
			"destinationDir": jx.Raw(`"/backup/photos-v2"`),
			"options":        jx.Raw(`{"sync":false}`),
		}),
	}, api.PeriodicJobsUpdateParams{ID: created.ID})
	if err != nil {
		t.Fatalf("PeriodicJobsUpdate failed: %v", err)
	}
	if updated.Name != "daily-photos-v2" || updated.CronExpression != "*/10 * * * *" {
		t.Fatalf("unexpected updated periodic job: %+v", updated)
	}

	if err := client.PeriodicJobsDisable(ctx, api.PeriodicJobsDisableParams{ID: created.ID}); err != nil {
		t.Fatalf("PeriodicJobsDisable failed: %v", err)
	}
	disabled, err := client.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams{ID: created.ID})
	if err != nil {
		t.Fatalf("PeriodicJobsGet after disable failed: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected periodic job to be disabled")
	}

	if err := client.PeriodicJobsEnable(ctx, api.PeriodicJobsEnableParams{ID: created.ID}); err != nil {
		t.Fatalf("PeriodicJobsEnable failed: %v", err)
	}

	run, err := client.PeriodicJobsRun(ctx, api.PeriodicJobsRunParams{ID: created.ID})
	if err != nil {
		t.Fatalf("PeriodicJobsRun failed: %v", err)
	}
	if run.Kind != "sync.run" {
		t.Fatalf("expected sync.run kind, got %s", run.Kind)
	}

	if err := client.PeriodicJobsDelete(ctx, api.PeriodicJobsDeleteParams{ID: created.ID}); err != nil {
		t.Fatalf("PeriodicJobsDelete failed: %v", err)
	}
	_, err = client.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams{ID: created.ID})
	if statusCode(err) != 404 {
		t.Fatalf("expected 404 after delete, got %d err=%v", statusCode(err), err)
	}
}

func TestPeriodicJobsRoutes_Validation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7312, "user7312")

	t.Run("invalid cron expression returns 400", func(t *testing.T) {
		_, err := client.PeriodicJobsCreate(ctx, &api.PeriodicJobCreate{
			Name:           "bad-cron",
			Enabled:        api.NewOptBool(true),
			CronExpression: "not-a-cron",
			Args: api.SyncArgs{
				Source:         "local:///tmp/source",
				DestinationDir: "/backup/x",
				Headers:        api.NewOptSyncArgsHeaders(api.SyncArgsHeaders{}),
				Proxy:          api.NewOptString(""),
			},
		})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("destinationDir must be absolute", func(t *testing.T) {
		_, err := client.PeriodicJobsCreate(ctx, &api.PeriodicJobCreate{
			Name:           "bad-destination",
			Enabled:        api.NewOptBool(true),
			CronExpression: "*/5 * * * *",
			Args: api.SyncArgs{
				Source:         "local:///tmp/source",
				DestinationDir: "relative/path",
				Headers:        api.NewOptSyncArgsHeaders(api.SyncArgsHeaders{}),
				Proxy:          api.NewOptString(""),
			},
		})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("maintenance jobs cannot be deleted", func(t *testing.T) {
		items, err := client.PeriodicJobsList(ctx)
		if err != nil {
			t.Fatalf("PeriodicJobsList failed: %v", err)
		}
		var maintenanceID api.UUID
		for _, item := range items {
			if item.Kind != api.PeriodicJobKindSyncRun {
				maintenanceID = item.ID
				break
			}
		}
		if uuid.UUID(maintenanceID) == uuid.Nil {
			t.Fatalf("expected a maintenance job")
		}
		if err := client.PeriodicJobsDelete(ctx, api.PeriodicJobsDeleteParams{ID: maintenanceID}); statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("maintenance retention can be updated", func(t *testing.T) {
		items, err := client.PeriodicJobsList(ctx)
		if err != nil {
			t.Fatalf("PeriodicJobsList failed: %v", err)
		}
		var staleUploadsID api.UUID
		for _, item := range items {
			if item.Kind == api.PeriodicJobKindCleanStaleUploads {
				staleUploadsID = item.ID
				break
			}
		}
		if uuid.UUID(staleUploadsID) == uuid.Nil {
			t.Fatalf("expected clean.stale_uploads maintenance job")
		}

		updated, err := client.PeriodicJobsUpdate(ctx, &api.PeriodicJobUpdate{
			Args: api.NewOptPeriodicJobUpdateArgs(api.PeriodicJobUpdateArgs{"retention": jx.Raw(`"2h"`)}),
		}, api.PeriodicJobsUpdateParams{ID: staleUploadsID})
		if err != nil {
			t.Fatalf("PeriodicJobsUpdate failed: %v", err)
		}
		args, ok := updated.Args.Get()
		if !ok {
			t.Fatalf("expected updated args")
		}
		retentionRaw, ok := args["retention"]
		if !ok {
			t.Fatalf("expected retention in args: %+v", args)
		}
		var retention string
		if err := json.Unmarshal(retentionRaw, &retention); err != nil {
			t.Fatalf("unmarshal retention: %v", err)
		}
		if retention != "2h0m0s" {
			t.Fatalf("expected retention=2h0m0s, got %s", retention)
		}
	})

	t.Run("pending files args cannot be updated", func(t *testing.T) {
		items, err := client.PeriodicJobsList(ctx)
		if err != nil {
			t.Fatalf("PeriodicJobsList failed: %v", err)
		}
		var pendingFilesID api.UUID
		for _, item := range items {
			if item.Kind == api.PeriodicJobKindCleanPendingFiles {
				pendingFilesID = item.ID
				break
			}
		}
		if uuid.UUID(pendingFilesID) == uuid.Nil {
			t.Fatalf("expected clean.pending_files maintenance job")
		}

		_, err = client.PeriodicJobsUpdate(ctx, &api.PeriodicJobUpdate{
			Args: api.NewOptPeriodicJobUpdateArgs(api.PeriodicJobUpdateArgs{"retention": jx.Raw(`"1h"`)}),
		}, api.PeriodicJobsUpdateParams{ID: pendingFilesID})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("run unknown periodic job returns 404", func(t *testing.T) {
		_, err := client.PeriodicJobsRun(ctx, api.PeriodicJobsRunParams{ID: api.UUID(uuid.MustParse("00000000-0000-0000-0000-000000000000"))})
		if statusCode(err) != 404 {
			t.Fatalf("expected 404, got %d err=%v", statusCode(err), err)
		}
	})
}
