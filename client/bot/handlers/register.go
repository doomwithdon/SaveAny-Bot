package handlers

import (
	"regexp"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	sabotfilters "github.com/krau/SaveAny-Bot/client/bot/handlers/utils/filters"
	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/re"
	userclient "github.com/krau/SaveAny-Bot/client/user"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/krau/SaveAny-Bot/pkg/tcbdata"
)

type DescCommandHandler struct {
	Cmd     string
	Desc    string
	handler func(ctx *ext.Context, u *ext.Update) error
}

var CommandHandlers = []DescCommandHandler{
	{"start", "开始使用", handleHelpCmd},
	{"silent", "切换静默模式", handleSilentCmd},
	{"storage", "设置默认存储端", handleStorageCmd},
	{"dir", "管理存储文件夹", handleDirCmd},
	{"rule", "管理自动存储规则", handleRuleCmd},
	{"watch", "监听聊天(UserBot)", handleWatchCmd},
	{"unwatch", "取消监听聊天(UserBot)", handleUnwatchCmd},
	{"save", "保存文件", handleSilentMode(handleSaveCmd, handleSilentSaveReplied)},
	{"config", "修改配置", handleConfigCmd},
	{"fnametmpl", "设置文件命名模板", handleConfigFnameTmpl},
	{"update", "检查更新", handleUpdateCmd},
	{"help", "显示帮助", handleHelpCmd},
}

func Register(disp dispatcher.Dispatcher) {
	disp.AddHandler(handlers.NewMessage(filters.Message.ChatType(filters.ChatTypeChannel), func(ctx *ext.Context, u *ext.Update) error {
		return dispatcher.EndGroups
	}))
	disp.AddHandler(handlers.NewMessage(filters.Message.ChatType(filters.ChatTypeChat), func(ctx *ext.Context, u *ext.Update) error {
		return dispatcher.EndGroups
	}))
	disp.AddHandler(handlers.NewMessage(filters.Message.All, checkPermission))
	for _, info := range CommandHandlers {
		disp.AddHandler(handlers.NewCommand(info.Cmd, info.handler))
	}
	disp.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("update"), handleUpdateCallback))
	disp.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix(tcbdata.TypeAdd), handleAddCallback))
	disp.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix(tcbdata.TypeSetDefault), handleSetDefaultCallback))
	disp.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix(tcbdata.TypeCancel), handleCancelCallback))
	disp.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix(tcbdata.TypeConfig), handleConfigCallback))
	disp.AddHandler(handlers.NewMessage(sabotfilters.RegexUrl(regexp.MustCompile(re.TgMessageLinkRegexString)), handleSilentMode(handleMessageLink, handleSilentSaveLink)))
	disp.AddHandler(handlers.NewMessage(sabotfilters.RegexUrl(regexp.MustCompile(re.TelegraphUrlRegexString)), handleSilentMode(handleTelegraphUrlMessage, handleSilentSaveTelegraph)))
	disp.AddHandler(handlers.NewMessage(filters.Message.Media, handleSilentMode(handleMediaMessage, handleSilentSaveMedia)))
	disp.AddHandler(handlers.NewMessage(filters.Message.Text, handleSilentMode(handleTextMessage, handleSilentSaveText)))

	if config.C().Telegram.Userbot.Enable {
		go listenMediaMessageEvent(userclient.GetMediaMessageCh())
	}
}
