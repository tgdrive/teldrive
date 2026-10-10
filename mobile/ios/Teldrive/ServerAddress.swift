import Foundation

enum ServerAddress {
    static func parse(_ value: String, allowLocalHTTP: Bool) throws -> URL {
        guard var parts = URLComponents(string: value.trimmingCharacters(in: .whitespacesAndNewlines)),
              let host = parts.host?.lowercased(), !host.isEmpty, let scheme = parts.scheme?.lowercased(),
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil,
              parts.path.isEmpty || parts.path == "/", parts.port == nil || (1...65535).contains(parts.port!) else {
            throw ConnectionError.invalidAddress
        }
        guard scheme == "https" || scheme == "http" && allowLocalHTTP && isLocal(host) else { throw ConnectionError.httpsRequired }
        parts.scheme = scheme; parts.host = host; parts.path = "/"
        guard let url = parts.url else { throw ConnectionError.invalidAddress }
        return url
    }
    static func isLocal(_ host: String) -> Bool {
        let host = host.lowercased().replacingOccurrences(of: "[", with: "").replacingOccurrences(of: "]", with: "")
        if host == "localhost" || host == "::1" || host.hasSuffix(".local") || host.contains(":") && (host.hasPrefix("fc") || host.hasPrefix("fd")) { return true }
        let parts = host.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 4, parts.allSatisfy({ !$0.isEmpty && $0.count <= 3 && $0.allSatisfy({ $0.isASCII && $0.isNumber }) }) else { return false }
        let values = parts.compactMap { Int($0) }
        guard values.count == 4, values.allSatisfy({ (0...255).contains($0) }) else { return false }
        return values[0] == 10 || values[0] == 127 || values[0] == 192 && values[1] == 168 || values[0] == 172 && (16...31).contains(values[1])
    }
    static func sameOrigin(_ first: URL, _ second: URL) -> Bool {
        first.scheme?.lowercased() == second.scheme?.lowercased() && first.host?.lowercased() == second.host?.lowercased()
            && (first.port ?? (first.scheme == "https" ? 443 : 80)) == (second.port ?? (second.scheme == "https" ? 443 : 80))
    }
    enum ConnectionError: LocalizedError {
        case invalidAddress, httpsRequired
        var errorDescription: String? {
            switch self {
            case .invalidAddress: return "Introduce únicamente la dirección del servidor, sin credenciales ni rutas."
            case .httpsRequired: return "Utiliza HTTPS. Puedes permitir HTTP para una dirección privada de tu red local."
            }
        }
    }
}
