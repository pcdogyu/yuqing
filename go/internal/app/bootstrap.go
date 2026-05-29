package app

import (
	"context"

	"github.com/go-resty/resty/v2"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10flash"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10xnews"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

func NewStore(cfg config.Config) (*sqlitestore.Store, error) {
	store, err := sqlitestore.New(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureDefaultAdmin(context.Background(), cfg.DefaultAdminUser, cfg.DefaultAdminPass); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.EnsureSeedData(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func NewCrawler(cfg config.Config, store *sqlitestore.Store) *service.Crawler {
	httpClient := resty.New().
		SetTimeout(cfg.HTTPTimeout).
		SetRetryCount(2).
		SetHeader("User-Agent", cfg.UserAgent)
	return service.NewCrawler(store, provider.Registry{
		Flash:    jin10flash.NewProvider(httpClient, cfg.FlashURL),
		Headline: jin10xnews.NewProvider(httpClient, cfg.HeadlineURL),
	})
}
