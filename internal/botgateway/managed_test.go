package botgateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

type managedInvoker func(context.Context, bin.Encoder, bin.Decoder) error

func (f managedInvoker) Invoke(ctx context.Context, in bin.Encoder, out bin.Decoder) error {
	return f(ctx, in, out)
}

type managedRunner struct{ invoke managedInvoker }

func (r managedRunner) Run(ctx context.Context, id int64, operation telegramstore.Operation, fn func(context.Context, *tg.Client) error) error {
	if id != 42 || operation != telegramstore.OperationManage {
		return bots.ErrInvalidInput
	}
	return fn(ctx, tg.NewClient(r.invoke))
}

func TestManagedCreationAndSafeRecovery(t *testing.T) {
	for _, mode := range []string{"new", "recover", "wrong-owner", "wrong-manager", "export-interrupted"} {
		t.Run(mode, func(t *testing.T) {
			created, exported := 0, 0
			manager := &tg.User{ID: 100, Username: "manager_bot", Bot: true, BotCanManageBots: true, AccessHash: 11}
			child := &tg.User{ID: 200, FirstName: "Storage", LastName: "principal", Username: "storage_bot", Bot: true, BotCanEdit: mode != "wrong-owner", AccessHash: 22}
			manager.SetAccessHash(11)
			child.SetAccessHash(22)
			userAPI := managedInvoker(func(_ context.Context, in bin.Encoder, out bin.Decoder) error {
				switch req := in.(type) {
				case *tg.ContactsResolveUsernameRequest:
					user := manager
					if req.Username == child.Username {
						user = child
					}
					out.(*tg.ContactsResolvedPeer).Users = []tg.UserClass{user}
				case *tg.BotsCheckUsernameRequest:
					if mode != "new" && mode != "export-interrupted" {
						return tgerr.New(400, "USERNAME_OCCUPIED")
					}
					out.(*tg.BoolBox).Bool = &tg.BoolTrue{}
				case *tg.BotsCreateBotRequest:
					created++
					if req.ManagerID.(*tg.InputUser).AccessHash != 11 || req.Name != "Storage" || req.Username != child.Username {
						t.Fatal("wrong creation parameters")
					}
					out.(*tg.UserBox).User = child
				case *tg.UsersGetFullUserRequest:
					managerID := manager.ID
					if mode == "wrong-manager" {
						managerID = 999
					}
					out.(*tg.UsersUserFull).FullUser.BotManagerID = managerID
				default:
					t.Fatalf("unexpected user request: %T", in)
				}
				return nil
			})
			managerAPI := managedInvoker(func(_ context.Context, in bin.Encoder, out bin.Decoder) error {
				switch req := in.(type) {
				case *tg.ContactsResolveUsernameRequest:
					copy := *child
					copy.AccessHash = 33
					copy.SetAccessHash(33)
					out.(*tg.ContactsResolvedPeer).Users = []tg.UserClass{&copy}
				case *tg.BotsExportBotTokenRequest:
					exported++
					if req.Revoke || req.Bot.(*tg.InputUser).AccessHash != 33 {
						t.Fatal("must keep tokens and use the manager's access hash")
					}
					if mode == "export-interrupted" {
						return errors.New("network failure with PRIVATE_MANAGER_TOKEN")
					}
					out.(*tg.BotsExportedBotToken).Token = "200:private-child-token"
				default:
					t.Fatalf("unexpected manager request: %T", in)
				}
				return nil
			})
			gateway := NewManaged(managedRunner{userAPI}, nil)
			gateway.runManager = func(ctx context.Context, _ string, fn func(context.Context, *tg.Client, *tg.User) error) error {
				return fn(ctx, tg.NewClient(managerAPI), manager)
			}
			result, err := gateway.CreateManaged(context.Background(), 42, "private-manager-token", "Storage", child.Username)
			if mode == "wrong-owner" || mode == "wrong-manager" {
				if !errors.Is(err, bots.ErrBotUsernameOccupied) || created != 0 || exported != 0 {
					t.Fatalf("unsafe recovery: %v, %d, %d", err, created, exported)
				}
				return
			}
			if mode == "export-interrupted" {
				if !errors.Is(err, bots.ErrManagedTelegram) || strings.Contains(err.Error(), "PRIVATE_MANAGER_TOKEN") {
					t.Fatalf("leaked RPC error: %v", err)
				}
				return
			}
			if err != nil || result.Name != "Storage principal" || result.Token != "200:private-child-token" || result.ID != 200 || result.Recovered != (mode == "recover") || exported != 1 {
				t.Fatalf("creation/recovery failed: %v", err)
			}
			if mode == "recover" && created != 0 {
				t.Fatal("recovery must not create another bot")
			}
		})
	}
}

func TestManagedPermissionAndTelegramErrors(t *testing.T) {
	gateway := NewManaged(nil, nil)
	gateway.runManager = func(ctx context.Context, _ string, fn func(context.Context, *tg.Client, *tg.User) error) error {
		return fn(ctx, nil, &tg.User{Bot: true})
	}
	if _, err := gateway.Manager(context.Background(), "secret"); !errors.Is(err, bots.ErrManagerPermission) {
		t.Fatal(err)
	}
	for _, test := range []struct {
		rpc  string
		want error
	}{
		{"USERNAME_OCCUPIED", bots.ErrBotUsernameOccupied}, {"BOT_CREATE_LIMIT_EXCEEDED", bots.ErrBotCreateLimit},
		{"MANAGER_PERMISSION_MISSING", bots.ErrManagerPermission}, {"FLOOD_WAIT_60", bots.ErrBotFloodWait},
	} {
		if !errors.Is(managedError(tgerr.New(400, test.rpc)), test.want) {
			t.Fatalf("wrong error for %s", test.rpc)
		}
	}
}
