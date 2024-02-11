package config

var defaultConfig = map[string]interface{}{
	"server.port":             5000,
	"server.readTimeout":      "15s",
	"server.writeTimeout":     "10s",
	"server.gracefulShutdown": "30s",

	"logging.level":       -1,
	"logging.encoding":    "console",
	"logging.development": true,

	"jwt.sessionTime": "720h",

	"db.dataSourceName":   "postgres://test:test@127.0.0.1:5432/teldrive",
	"db.logLevel":         1,
	"db.migrate.enable":   true,
	"db.pool.maxOpen":     25,
	"db.pool.maxIdle":     25,
	"db.pool.maxLifetime": "10m",

	"telegram.rateLimit":         true,
	"telegram.rateBurst":         5,
	"telegram.rate":              100,
	"telegram.deviceModel":       "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0",
	"telegram.systemVersion":     "Win32",
	"telegram.appVersion":        "2.1.9 K",
	"telegram.langCode":          "en",
	"telegram.systemLangCode":    "en-US",
	"telegram.langPack":          "en",
	"telegram.bgBotsLimit":       5,
	"telegram.disableStreamBots": false,
	"telegram.uploads.threads":   16,
	"telegram.uploads.retention": "210h",
}
