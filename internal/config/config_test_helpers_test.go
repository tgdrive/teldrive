package config

func validTestConfig() Config {
	cfg := Default()
	cfg.Database.URL = "postgres://example/teldrive"
	cfg.Telegram.AppID = 12345
	cfg.Telegram.AppHash = "telegram-app-hash"
	cfg.Security.SigningKey = "0123456789abcdef0123456789abcdef"
	cfg.Security.DataKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	return cfg
}
