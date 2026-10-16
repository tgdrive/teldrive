package botgateway

import (
	"context"
	"errors"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

type Managed struct {
	users   telegramstore.Runner
	factory *telegramstore.Factory
	// Override the authenticated bot lifetime in protocol tests.
	runManager func(context.Context, string, func(context.Context, *tg.Client, *tg.User) error) error
}

func NewManaged(users telegramstore.Runner, factory *telegramstore.Factory) *Managed {
	return &Managed{users: users, factory: factory}
}

func managedError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	for code, friendly := range map[string]error{
		"USERNAME_OCCUPIED": bots.ErrBotUsernameOccupied,
		"USERNAME_INVALID":  bots.ErrBotUsernameInvalid, "USERNAME_SUFFIX_MISSING": bots.ErrBotUsernameInvalid,
		"BOT_CREATE_LIMIT_EXCEEDED":  bots.ErrBotCreateLimit,
		"MANAGER_PERMISSION_MISSING": bots.ErrManagerPermission, "MANAGER_INVALID": bots.ErrManagerPermission,
		"BOT_TOKEN_INVALID": bots.ErrManagerPermission, "ACCESS_TOKEN_INVALID": bots.ErrManagerPermission,
	} {
		if tgerr.Is(err, code) {
			return friendly
		}
	}
	if _, ok := tgerr.AsFloodWait(err); ok {
		return bots.ErrBotFloodWait
	}
	for _, known := range []error{bots.ErrManagedUnavailable, bots.ErrManagerPermission, bots.ErrBotUsernameOccupied, bots.ErrNotBot, bots.ErrInvalidInput} {
		if errors.Is(err, known) {
			return known
		}
	}
	// RPC/authentication errors must never include tokens in logs or API responses.
	return bots.ErrManagedTelegram
}

func (g *Managed) withManager(ctx context.Context, token string, fn func(context.Context, *tg.Client, *tg.User) error) error {
	callback := func(ctx context.Context, api *tg.Client, self *tg.User) error {
		if self == nil || !self.Bot || !self.BotCanManageBots || self.Username == "" {
			return bots.ErrManagerPermission
		}
		return fn(ctx, api, self)
	}
	if g.runManager != nil {
		return g.runManager(ctx, token, callback)
	}
	if g.factory == nil {
		return bots.ErrManagedUnavailable
	}
	client, err := g.factory.New(&session.StorageMemory{})
	if err != nil {
		return err
	}
	return client.Run(ctx, func(ctx context.Context) error {
		if _, err := client.Auth().Bot(ctx, token); err != nil {
			return err
		}
		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		return callback(ctx, client.API(), self)
	})
}

func (g *Managed) Manager(ctx context.Context, token string) (bots.Identity, error) {
	var result bots.Identity
	err := g.withManager(ctx, token, func(_ context.Context, _ *tg.Client, self *tg.User) error {
		result = bots.Identity{ID: self.ID, Name: strings.TrimSpace(self.FirstName + " " + self.LastName), Username: self.Username}
		return nil
	})
	return result, managedError(err)
}

func (g *Managed) CheckUsername(ctx context.Context, userID int64, username string) error {
	if g.users == nil || userID <= 0 {
		return bots.ErrManagedUnavailable
	}
	err := g.users.Run(ctx, userID, telegramstore.OperationManage, func(ctx context.Context, api *tg.Client) error {
		available, err := api.BotsCheckUsername(ctx, username)
		if err != nil {
			return err
		}
		if !available {
			return bots.ErrBotUsernameOccupied
		}
		return nil
	})
	return managedError(err)
}

func resolveManagedUser(ctx context.Context, api *tg.Client, username string) (*tg.User, error) {
	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, err
	}
	for _, value := range resolved.Users {
		if user, ok := value.(*tg.User); ok && user.Bot && strings.EqualFold(user.Username, username) {
			return user, nil
		}
	}
	return nil, bots.ErrNotBot
}

func (g *Managed) CreateManaged(ctx context.Context, userID int64, token, name, username string) (bots.ManagedIdentity, error) {
	if g.users == nil || userID <= 0 {
		return bots.ManagedIdentity{}, bots.ErrManagedUnavailable
	}
	var result bots.ManagedIdentity
	err := g.withManager(ctx, token, func(ctx context.Context, managerAPI *tg.Client, manager *tg.User) error {
		var child *tg.User
		err := g.users.Run(ctx, userID, telegramstore.OperationManage, func(ctx context.Context, api *tg.Client) error {
			peer, err := resolveManagedUser(ctx, api, manager.Username)
			if err != nil {
				return err
			}
			if peer.ID != manager.ID || !peer.BotCanManageBots {
				return bots.ErrManagerPermission
			}
			available, err := api.BotsCheckUsername(ctx, username)
			if err != nil && !tgerr.Is(err, "USERNAME_OCCUPIED") {
				return err
			}
			if err == nil && available {
				created, createErr := api.BotsCreateBot(ctx, &tg.BotsCreateBotRequest{Name: name, Username: username, ManagerID: peer.AsInput()})
				if createErr == nil {
					var ok bool
					child, ok = created.(*tg.User)
					if !ok || !child.Bot || !strings.EqualFold(child.Username, username) {
						return bots.ErrNotBot
					}
					return nil
				}
				if !tgerr.Is(createErr, "USERNAME_OCCUPIED") {
					return createErr
				}
			}
			// Resume only a bot owned by this user and managed by this manager.
			child, err = resolveManagedUser(ctx, api, username)
			if err != nil || !child.BotCanEdit {
				return bots.ErrBotUsernameOccupied
			}
			full, err := api.UsersGetFullUser(ctx, child.AsInput())
			if err != nil {
				return err
			}
			if full.FullUser.BotManagerID != manager.ID {
				return bots.ErrBotUsernameOccupied
			}
			result.Recovered = true
			return nil
		})
		if err != nil {
			return err
		}
		// Access hashes belong to the logged-in account: resolve again as manager.
		managed, err := resolveManagedUser(ctx, managerAPI, username)
		if err != nil {
			return err
		}
		if managed.ID != child.ID {
			return bots.ErrNotBot
		}
		exported, err := managerAPI.BotsExportBotToken(ctx, &tg.BotsExportBotTokenRequest{Bot: managed.AsInput(), Revoke: false})
		if err != nil {
			return err
		}
		result.Identity = bots.Identity{ID: child.ID, Name: strings.TrimSpace(child.FirstName + " " + child.LastName), Username: child.Username}
		result.Token = exported.Token
		return nil
	})
	return result, managedError(err)
}
