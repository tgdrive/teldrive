package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gotd/contrib/storage"
	"github.com/tgdrive/teldrive/internal/api"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/pkg/services"
)

func TestUsersRoutes_BasicEndpoints(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7206, "user7206")

	if err := client.UsersUpdateChannel(ctx, &api.ChannelUpdate{ChannelId: api.NewOptInt64(900050), ChannelName: api.NewOptString("primary")}); err != nil {
		t.Fatalf("UsersUpdateChannel failed: %v", err)
	}
	if _, err := client.UsersStats(ctx); err != nil {
		t.Fatalf("UsersStats failed: %v", err)
	}
	if _, err := client.UsersListSessions(ctx); err != nil {
		t.Fatalf("UsersListSessions failed: %v", err)
	}
	if _, err := client.UsersListChannels(ctx); err != nil {
		t.Fatalf("UsersListChannels failed: %v", err)
	}
	if _, err := client.UsersProfileImage(ctx); err != nil {
		t.Fatalf("UsersProfileImage failed: %v", err)
	}
	if err := client.UsersRemoveBots(ctx); err != nil {
		t.Fatalf("UsersRemoveBots failed: %v", err)
	}
}

func TestUsersRoutes_ValidationAndRollback(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7002, "user7002")

	t.Run("UsersDeleteChannel invalid id => 400", func(t *testing.T) {
		err := client.UsersDeleteChannel(ctx, api.UsersDeleteChannelParams{ID: "not-number"})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("UsersUpdateChannel missing channelId => 400", func(t *testing.T) {
		err := client.UsersUpdateChannel(ctx, &api.ChannelUpdate{})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("UsersUpdateChannel rolls back on mid-transaction failure", func(t *testing.T) {
		const (
			channelA = int64(910101)
			channelB = int64(910202)
		)

		selectedTrue := true
		selectedFalse := false

		if err := s.repos.Channels.Create(ctx, &jetmodel.Channels{UserID: 7002, ChannelID: channelA, ChannelName: "a", Selected: &selectedTrue}); err != nil {
			t.Fatalf("seed channelA: %v", err)
		}
		if err := s.repos.Channels.Create(ctx, &jetmodel.Channels{UserID: 7002, ChannelID: channelB, ChannelName: "b", Selected: &selectedFalse}); err != nil {
			t.Fatalf("seed channelB: %v", err)
		}

		_, err := s.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_fail_update_channel_selected ON teldrive.channels")
		if err != nil {
			t.Fatalf("drop old trigger: %v", err)
		}
		_, err = s.pool.Exec(ctx, "DROP FUNCTION IF EXISTS teldrive.test_fail_update_channel_selected()")
		if err != nil {
			t.Fatalf("drop old trigger function: %v", err)
		}

		_, err = s.pool.Exec(ctx, `
			CREATE FUNCTION teldrive.test_fail_update_channel_selected()
			RETURNS trigger AS $$
			BEGIN
				IF NEW.channel_id = 910202 AND NEW.selected = true THEN
					RAISE EXCEPTION 'forced channel update failure';
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;
		`)
		if err != nil {
			t.Fatalf("create trigger function: %v", err)
		}

		_, err = s.pool.Exec(ctx, `
			CREATE TRIGGER test_fail_update_channel_selected
			BEFORE UPDATE ON teldrive.channels
			FOR EACH ROW
			EXECUTE FUNCTION teldrive.test_fail_update_channel_selected();
		`)
		if err != nil {
			t.Fatalf("create trigger: %v", err)
		}

		t.Cleanup(func() {
			_, _ = s.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_fail_update_channel_selected ON teldrive.channels")
			_, _ = s.pool.Exec(ctx, "DROP FUNCTION IF EXISTS teldrive.test_fail_update_channel_selected()")
		})

		err = client.UsersUpdateChannel(ctx, &api.ChannelUpdate{ChannelId: api.NewOptInt64(channelB), ChannelName: api.NewOptString("b")})
		if statusCode(err) != 500 {
			t.Fatalf("expected 500, got %d err=%v", statusCode(err), err)
		}

		postA, err := s.repos.Channels.GetByChannelID(ctx, channelA)
		if err != nil {
			t.Fatalf("fetch channelA after rollback: %v", err)
		}
		postB, err := s.repos.Channels.GetByChannelID(ctx, channelB)
		if err != nil {
			t.Fatalf("fetch channelB after rollback: %v", err)
		}

		if postA.Selected == nil || !*postA.Selected {
			t.Fatalf("expected channelA selected=true after rollback, got %+v", postA.Selected)
		}
		if postB.Selected == nil || *postB.Selected {
			t.Fatalf("expected channelB selected=false after rollback, got %+v", postB.Selected)
		}
	})

	t.Run("UsersRemoveSession foreign session => 404", func(t *testing.T) {
		_, _, foreignHash := loginWithClient(t, s, 7003, "user7003")
		err := client.UsersRemoveSession(ctx, api.UsersRemoveSessionParams{ID: api.UUID(uuid.MustParse(foreignHash))})
		if statusCode(err) != 404 {
			t.Fatalf("expected 404, got %d err=%v", statusCode(err), err)
		}
	})
}

func TestUsersRoutes_TelegramFailureScenarios(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, sessionHash := loginWithClient(t, s, 7208, "user7208")

	t.Run("UsersDeleteChannel telegram delete failure => 500", func(t *testing.T) {
		selected := true
		if err := s.repos.Channels.Create(ctx, &jetmodel.Channels{UserID: 7208, ChannelID: 920001, ChannelName: "to-delete", Selected: &selected}); err != nil {
			t.Fatalf("seed channel: %v", err)
		}
		s.tgMock.deleteChannelFn = func(context.Context, services.TelegramClient, int64) (storage.PeerKey, error) {
			return storage.PeerKey{}, errors.New("delete channel failed")
		}
		err := client.UsersDeleteChannel(ctx, api.UsersDeleteChannelParams{ID: "920001"})
		if statusCode(err) != 500 {
			t.Fatalf("expected 500, got %d err=%v", statusCode(err), err)
		}
		s.tgMock.deleteChannelFn = nil
	})

	t.Run("UsersSyncChannels auth client failure => 500", func(t *testing.T) {
		s.tgMock.authClientFn = func(context.Context, string, int) (services.TelegramClient, error) {
			return nil, errors.New("auth client failed")
		}
		err := client.UsersSyncChannels(ctx)
		if statusCode(err) != 500 {
			t.Fatalf("expected 500, got %d err=%v", statusCode(err), err)
		}
		s.tgMock.authClientFn = nil
	})

	t.Run("UsersListSessions authorizations failure => 500", func(t *testing.T) {
		s.tgMock.listAuthsFn = func(context.Context, services.TelegramClient) ([]services.TelegramAuthorization, error) {
			return nil, errors.New("list auths failed")
		}
		_, err := client.UsersListSessions(ctx)
		if statusCode(err) != 500 {
			t.Fatalf("expected 500, got %d err=%v", statusCode(err), err)
		}
		s.tgMock.listAuthsFn = nil
	})

	t.Run("UsersProfileImage telegram failure => 500", func(t *testing.T) {
		s.tgMock.profilePhotoFn = func(context.Context, services.TelegramClient) ([]byte, int64, bool, error) {
			return nil, 0, false, errors.New("profile failed")
		}
		_, err := client.UsersProfileImage(ctx)
		if statusCode(err) != 500 {
			t.Fatalf("expected 500, got %d err=%v", statusCode(err), err)
		}
		s.tgMock.profilePhotoFn = nil
	})

	t.Run("UsersRemoveSession logout failure still revokes session", func(t *testing.T) {
		s.tgMock.logoutFn = func(context.Context, services.TelegramClient) error {
			return errors.New("logout failed")
		}
		err := client.UsersRemoveSession(ctx, api.UsersRemoveSessionParams{ID: api.UUID(uuid.MustParse(sessionHash))})
		if statusCode(err) != 200 {
			t.Fatalf("expected 200, got %d err=%v", statusCode(err), err)
		}
		if _, getErr := s.repos.Sessions.GetByID(ctx, uuid.MustParse(sessionHash)); getErr == nil {
			t.Fatalf("expected session to be revoked despite telegram logout failure")
		}
		s.tgMock.logoutFn = nil
	})
}
