package bot

import (
	"context"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/charmbracelet/log"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/krau/SaveAny-Bot/client/bot/handlers"
	"github.com/krau/SaveAny-Bot/client/middleware"
	"github.com/krau/SaveAny-Bot/common/utils/netutil"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/ncruces/go-sqlite3/gormlite"
	"golang.org/x/net/proxy"
)

func Init(ctx context.Context) <-chan struct{} {
	log.FromContext(ctx).Info("初始化 Bot...")
	resultChan := make(chan struct {
		client *gotgproto.Client
		err    error
	})
	shouldRestart := make(chan struct{})
	go func() {
		var resolver dcs.Resolver
		if config.C().Telegram.Proxy.Enable && config.C().Telegram.Proxy.URL != "" {
			dialer, err := netutil.NewProxyDialer(config.C().Telegram.Proxy.URL)
			if err != nil {
				resultChan <- struct {
					client *gotgproto.Client
					err    error
				}{nil, err}
				return
			}
			resolver = dcs.Plain(dcs.PlainOptions{
				Dial: dialer.(proxy.ContextDialer).DialContext,
			})
		} else {
			resolver = dcs.DefaultResolver()
		}
		client, err := gotgproto.NewClient(
			config.C().Telegram.AppID,
			config.C().Telegram.AppHash,
			gotgproto.ClientTypeBot(config.C().Telegram.Token),
			&gotgproto.ClientOpts{
				Session:          sessionMaker.SqlSession(gormlite.Open(config.C().DB.Session)),
				DisableCopyright: true,
				Middlewares:      middleware.NewDefaultMiddlewares(ctx, 5*time.Minute),
				Resolver:         resolver,
				Context:          ctx,
				MaxRetries:       config.C().Telegram.RpcRetry,
				AutoFetchReply:   true,
				ErrorHandler: func(ctx *ext.Context, u *ext.Update, s string) error {
					if s == "SAVEANTBOT-RESTART" {
						shouldRestart <- struct{}{}
						return dispatcher.EndGroups
					}
					log.FromContext(ctx).Errorf("unhandled error: %s", s)
					return dispatcher.EndGroups
				},
			},
		)
		if err != nil {
			resultChan <- struct {
				client *gotgproto.Client
				err    error
			}{nil, err}
			return
		}
		client.API().BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
			Scope: &tg.BotCommandScopeDefault{},
		})
		commands := make([]tg.BotCommand, 0, len(handlers.CommandHandlers))
		for _, info := range handlers.CommandHandlers {
			commands = append(commands, tg.BotCommand{Command: info.Cmd, Description: info.Desc})
		}
		_, err = client.API().BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
			Scope:    &tg.BotCommandScopeDefault{},
			Commands: commands,
		})
		resultChan <- struct {
			client *gotgproto.Client
			err    error
		}{client, err}
	}()

	select {
	case <-ctx.Done():
		log.FromContext(ctx).Errorf("已取消 Bot 初始化: %s", ctx.Err())
	case result := <-resultChan:
		if result.err != nil {
			log.FromContext(ctx).Fatalf("初始化 Bot 失败: %s", result.err)
		}
		handlers.Register(result.client.Dispatcher)
		log.FromContext(ctx).Info("Bot 初始化完成")
	}
	return shouldRestart
}
