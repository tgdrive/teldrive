package cache

// User Keys
func KeyUserChannel(userID int64) string {
	return Key("users", "channel", userID)
}

func KeyUserBots(userID int64) string {
	return Key("users", "bots", userID)
}

func KeyUserSessions(userID int64) string {
	return Key("users", "sessions", userID)
}

// File Keys
func KeyFile(fileID string) string {
	return Key("files", fileID)
}

func KeyFileMessages(fileID string) string {
	return Key("files", "messages", fileID)
}

func KeyFileLocation(instance, botID, fileID string, partID any) string {
	return Key("files", "location", "bot", "instance", fileID, partID, botID, instance)
}

func KeyFileLocationPattern(fileID string) string {
	return Key("files", "location", "bot", "instance", fileID, "*")
}

// Session Keys
func KeySessionHash(hash string) string {
	return Key("sessions", hash)
}

func KeySessionToken(instance, token string) string {
	return Key("sessions", instance, token)
}

// Share Keys
func KeyShare(shareID string) string {
	return Key("shares", shareID)
}

// Peer Keys
func KeyPeer(userID int64) string {
	return Key("peers", userID)
}
