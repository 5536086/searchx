package run

import (
	"github.com/fatih/color"
	"github.com/iyear/searchx/app/bot/run/internal/config"
	"github.com/iyear/searchx/app/bot/run/internal/i18n"
	"github.com/iyear/searchx/app/bot/run/internal/middleware"
	"github.com/iyear/searchx/app/bot/run/internal/model"
	"github.com/iyear/searchx/global"
	"github.com/iyear/searchx/pkg/logger"
	"github.com/iyear/searchx/pkg/storage"
	"github.com/iyear/searchx/pkg/utils"
	tele "gopkg.in/telebot.v3"
	"log"
	"net/http"
	"time"
)

func Run(cfg string) {
	color.Blue(global.Logo)
	color.Blue("Initializing...")

	if err := config.Init(cfg); err != nil {
		log.Fatalf("init config failed: %v", err)
	}
	color.Blue("Config loaded")

	slog := logger.New(config.C.Log.Enable, "log/bot/latest.log", config.C.Log.Level)

	if err := i18n.Init(config.C.Ctrl.I18N); err != nil {
		slog.Fatalw("init i18n templates failed", "err", err)
	}
	color.Blue("I18n templates loaded")

	_kv, err := storage.NewKV(config.C.Storage.KV.Driver, config.C.Storage.KV.Options)
	if err != nil {
		slog.Fatalw("init kv database failed", "err", err, "options", config.C.Storage.KV.Options)
	}
	color.Blue("KV database initialized")

	_search, err := storage.NewSearch(config.C.Storage.Search.Driver, config.C.Storage.Search.Options)
	if err != nil {
		slog.Fatalw("init search engine database failed", "err", err, "options", config.C.Storage.Search.Options)
	}
	color.Blue("Search engine initialized")

	_cache, err := storage.New(config.C.Storage.Cache.Driver, config.C.Storage.Cache.Options)
	if err != nil {
		slog.Fatalw("init cache failed", "err", err, "options", config.C.Storage.Cache.Options)
	}
	color.Blue("Cache initialized")

	dialer, err := utils.ProxyFromURL(config.C.Proxy)
	if err != nil {
		slog.Fatalw("init proxy failed", "err", err, "proxy", config.C.Proxy)
	}

	settings := tele.Settings{
		Token:     config.C.Bot.Token,
		Poller:    &tele.LongPoller{Timeout: 5 * time.Second},
		Client:    &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}},
		OnError:   middleware.OnError(),
		ParseMode: tele.ModeMarkdown,
	}

	bot, err := tele.NewBot(settings)
	if err != nil {
		slog.Fatalw("create bot failed", "err", err)
	}

	color.Blue("Bot: %s", bot.Me.Username)

	scope := &model.Scope{
		Storage: &model.Storage{
			KV:     _kv,
			Search: _search,
			Cache:  _cache,
		},
		Log: slog.Named("bot"),
	}

	bot.Use(middleware.SetScope(scope), middleware.AutoResponder())

	makeHandlers(bot, i18n.Templates[config.C.Ctrl.DefaultLanguage].Button)

	bot.Start()
}
