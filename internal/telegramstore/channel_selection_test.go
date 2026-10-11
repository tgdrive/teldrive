package telegramstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

type channelSelectionRunner struct{ api *tg.Client }

func (r channelSelectionRunner) Run(ctx context.Context, _ int64, _ Operation, fn func(context.Context, *tg.Client) error) error {
	return fn(ctx, r.api)
}

type channelSelectionInvoker struct {
	chats      []tg.ChatClass
	adminCalls int
}

func (i *channelSelectionInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetDialogsRequest:
		dialogs := []tg.DialogClass{}
		for _, chat := range i.chats {
			var peer tg.PeerClass
			switch chat := chat.(type) {
			case *tg.Channel:
				peer = &tg.PeerChannel{ChannelID: chat.ID}
			case *tg.Chat:
				peer = &tg.PeerChat{ChatID: chat.ID}
			}
			dialogs = append(dialogs, &tg.Dialog{Peer: peer})
		}
		output.(*tg.MessagesDialogsBox).Dialogs = &tg.MessagesDialogs{Dialogs: dialogs, Chats: i.chats}
	case *tg.ChannelsGetChannelsRequest:
		output.(*tg.MessagesChatsBox).Chats = &tg.MessagesChats{Chats: i.chats}
	case *tg.ChannelsCreateChannelRequest:
		output.(*tg.UpdatesBox).Updates = &tg.Updates{Chats: i.chats}
	case *tg.ContactsResolveUsernameRequest:
		*output.(*tg.ContactsResolvedPeer) = tg.ContactsResolvedPeer{
			Users: []tg.UserClass{&tg.User{ID: 777, Bot: true, Username: "storage_bot"}},
		}
	case *tg.ChannelsEditAdminRequest:
		i.adminCalls++
		output.(*tg.UpdatesBox).Updates = &tg.Updates{}
	default:
		return fmt.Errorf("unexpected RPC %T", input)
	}
	return nil
}

func TestDiscoveryExcludesGroupsAndSupergroups(t *testing.T) {
	invoker := &channelSelectionInvoker{chats: []tg.ChatClass{
		&tg.Channel{ID: 1, Title: "storage", Creator: true, Broadcast: true},
		&tg.Channel{ID: 2, Title: "supergroup", Creator: true, Megagroup: true},
		&tg.Channel{ID: 3, Title: "gigagroup", Creator: true, Gigagroup: true},
		&tg.Chat{ID: 4, Title: "basic group", Creator: true},
		&tg.Channel{ID: 5, Title: "not manageable", Broadcast: true},
		&tg.Channel{ID: 6, Title: "left", Creator: true, Broadcast: true, Left: true},
	}}
	account, err := NewGotdAccount(channelSelectionRunner{api: tg.NewClient(invoker)})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := account.DiscoverChannels(context.Background(), 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].ID != 1 {
		t.Fatalf("discovered channels = %+v, want only broadcast channel 1", channels)
	}
}

func TestInviteBotSkipsPreviouslyRegisteredGroups(t *testing.T) {
	for _, channel := range []*tg.Channel{
		{ID: 1, Broadcast: true},
		{ID: 1, Megagroup: true},
		{ID: 1, Gigagroup: true},
	} {
		t.Run(fmt.Sprintf("broadcast=%v,mega=%v,giga=%v", channel.Broadcast, channel.Megagroup, channel.Gigagroup), func(t *testing.T) {
			invoker := &channelSelectionInvoker{chats: []tg.ChatClass{channel}}
			storage := &GotdStorage{runner: channelSelectionRunner{api: tg.NewClient(invoker)}}
			if err := storage.InviteBot(context.Background(), 1001, 1, "storage_bot"); err != nil {
				t.Fatal(err)
			}
			want := 0
			if channel.Broadcast {
				want = 1
			}
			if invoker.adminCalls != want {
				t.Fatalf("admin calls = %d, want %d", invoker.adminCalls, want)
			}
		})
	}
}

func TestInviteBotDoesNotModifyBasicGroup(t *testing.T) {
	invoker := &channelSelectionInvoker{chats: []tg.ChatClass{&tg.Chat{ID: 1, Creator: true}}}
	storage := &GotdStorage{runner: channelSelectionRunner{api: tg.NewClient(invoker)}}
	if err := storage.InviteBot(context.Background(), 1001, 1, "storage_bot"); err == nil {
		t.Fatal("basic group accepted as a channel")
	}
	if invoker.adminCalls != 0 {
		t.Fatal("bot added to basic group")
	}
}

func TestCreateChannelUsesOnlyBroadcastChannelFromUpdates(t *testing.T) {
	invoker := &channelSelectionInvoker{chats: []tg.ChatClass{
		&tg.Channel{ID: 1, Title: "group", Megagroup: true},
		&tg.Channel{ID: 2, Title: "storage", Broadcast: true},
	}}
	storage := &GotdStorage{runner: channelSelectionRunner{api: tg.NewClient(invoker)}}
	channel, err := storage.CreateChannel(context.Background(), 1001, "storage")
	if err != nil {
		t.Fatal(err)
	}
	if channel.ID != 2 {
		t.Fatalf("created channel = %+v, want broadcast channel 2", channel)
	}
}
